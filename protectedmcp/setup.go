package protectedmcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type selectedOperation struct {
	VersionID string `json:"operationVersionId"`
	Contract  struct {
		Summary      string         `json:"summary"`
		InputSchema  map[string]any `json:"inputSchema"`
		ResultSchema map[string]any `json:"resultSchema"`
		Coverage     []string       `json:"requiredCoverage"`
		Idempotency  string         `json:"idempotency"`
		Effects      []struct {
			Kind string `json:"kind"`
		} `json:"expectedEffects"`
		Resource *struct {
			Field []string `json:"field"`
		} `json:"resourceIdentity"`
	} `json:"contract"`
}
type selectedOperations struct {
	FormatVersion  int                 `json:"formatVersion"`
	OrganisationID string              `json:"organisationId"`
	DeploymentID   string              `json:"deploymentId"`
	BundleID       string              `json:"bundleId"`
	Sequence       int64               `json:"sequence"`
	Mode           string              `json:"mode"`
	ExpiresAt      time.Time           `json:"expiresAt"`
	Operations     []selectedOperation `json:"operations"`
}

// OperationResultSchema describes the actual object checked before an MCP result is released.
func OperationResultSchema(tool *mcp.Tool) map[string]any {
	data := tool.OutputSchema
	if data == nil {
		data = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}, "data": data}, "required": []string{"text", "data"}}
}
func selectedForSetup(ctx context.Context, settings Settings) (selectedOperations, error) {
	var result selectedOperations
	address, err := url.Parse(settings.SidecarURL)
	if err != nil || address.Scheme != "http" || address.User != nil || address.RawQuery != "" || address.Fragment != "" || address.Opaque != "" || (address.Path != "" && address.Path != "/") || !slices.Contains([]string{"127.0.0.1", "localhost", "::1"}, address.Hostname()) {
		return result, errors.New("setup needs the private loopback sidecar address")
	}
	token, err := privateToken(filepath.Join(settings.AuthRailDirectory, "adapter-token"))
	if err != nil {
		return result, err
	}
	connection, err := readPrivate(filepath.Join(settings.AuthRailDirectory, "connection.json"), 16384)
	if err != nil {
		return result, err
	}
	var identity struct {
		OrganisationID string `json:"organisationId"`
		DeploymentID   string `json:"deploymentId"`
	}
	if json.Unmarshal(connection, &identity) != nil || !uuid.MatchString(identity.OrganisationID) || !uuid.MatchString(identity.DeploymentID) {
		return result, errors.New("saved deployment identity is invalid")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(settings.SidecarURL, "/")+"/integration-operations", nil)
	if err != nil {
		return result, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return result, errors.New("sidecar setup contracts are unavailable")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 512*1024+1))
	if err != nil || len(body) > 512*1024 || response.StatusCode != 200 || json.Unmarshal(body, &result) != nil || result.FormatVersion != 1 || result.OrganisationID != identity.OrganisationID || result.DeploymentID != identity.DeploymentID || !uuid.MatchString(result.BundleID) || result.Sequence < 1 || result.Mode != "enforce" || !result.ExpiresAt.After(time.Now()) || len(result.Operations) == 0 || len(result.Operations) > 100 {
		return result, errors.New("select valid reviewed Enforce rules for this deployment before binding tools")
	}
	return result, nil
}

// PrepareBindings matches exact reviewed selected contracts without running a business tool or asking the user to wire an SDK.
func PrepareBindings(ctx context.Context, settings Settings, downstream ToolClient) (Settings, error) {
	selected, err := selectedForSetup(ctx, settings)
	if err != nil {
		return settings, err
	}
	available, err := discoverTools(ctx, downstream)
	if err != nil {
		return settings, err
	}
	if len(settings.Bindings) == 0 || len(settings.Bindings) > 64 {
		return settings, errors.New("choose the business tools to protect")
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return settings, err
	}
	var prepared Settings
	if json.Unmarshal(encoded, &prepared) != nil {
		return settings, errors.New("tool choices are invalid")
	}
	for i, binding := range prepared.Bindings {
		tool := available[binding.DownstreamName]
		if tool == nil {
			return settings, errors.New("a selected downstream tool is missing")
		}
		input, err := ReviewedInputSchema(tool.InputSchema, binding.IdempotencyField)
		if err != nil {
			return settings, err
		}
		inputHash, _ := hashJSON(input)
		resultHash, _ := hashJSON(OperationResultSchema(tool))
		matches := []selectedOperation{}
		for _, operation := range selected.Operations {
			if binding.OperationVersionID != "" && binding.OperationVersionID != operation.VersionID {
				continue
			}
			wantInput, _ := hashJSON(operation.Contract.InputSchema)
			wantResult, _ := hashJSON(operation.Contract.ResultSchema)
			if inputHash != wantInput || resultHash != wantResult || !slices.Contains(operation.Contract.Coverage, "tool_call") || !slices.Contains(operation.Contract.Coverage, "tool_result") {
				continue
			}
			writes := false
			for _, effect := range operation.Contract.Effects {
				writes = writes || effect.Kind != "read"
			}
			if writes == binding.ReadOnly || writes && (binding.IdempotencyField == "" || operation.Contract.Idempotency != "required") {
				continue
			}
			resource := ""
			if operation.Contract.Resource != nil {
				if len(operation.Contract.Resource.Field) != 1 {
					continue
				}
				resource = operation.Contract.Resource.Field[0]
			}
			if resource != binding.ResourceField {
				continue
			}
			if !uuid.MatchString(operation.VersionID) {
				return settings, errors.New("selected operation identity is invalid")
			}
			matches = append(matches, operation)
		}
		if len(matches) != 1 {
			return settings, errors.New("each tool needs one exact confirmed action selected in this deployment; resolve missing or ambiguous contracts before binding")
		}
		operation := matches[0]
		prepared.Bindings[i].OperationVersionID = operation.VersionID
		pin, err := SchemaDigest(tool)
		if err != nil {
			return settings, err
		}
		if binding.SchemaSHA256 != "" && binding.SchemaSHA256 != pin {
			return settings, errors.New("the reviewed downstream schema changed")
		}
		prepared.Bindings[i].SchemaSHA256 = pin
		if binding.Description == "" {
			prepared.Bindings[i].Description = operation.Contract.Summary
		}
	}
	// Reuse execution validation and private-state ownership before producing a runnable configuration.
	connector, err := Open(ctx, prepared, downstream)
	if err != nil {
		return settings, err
	}
	if err = connector.Close(); err != nil {
		return settings, err
	}
	return prepared, nil
}

// SavePreparedSettings creates a new private file; it never replaces an installed configuration.
func SavePreparedSettings(path string, settings Settings) error {
	if !filepath.IsAbs(path) {
		return errors.New("prepared configuration needs an absolute private path")
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("prepared configuration directory must be private")
	}
	body, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("configuration already exists or cannot be created; no file was replaced")
	}
	_, err = file.Write(append(body, '\n'))
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return errors.New("prepared configuration could not be retained")
	}
	directory, err := os.Open(parent)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
