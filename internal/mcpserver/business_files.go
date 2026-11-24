package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type businessFilePrepareInput struct {
	RouteID        string         `json:"routeId"`
	BindingSHA256  string         `json:"bindingSha256"`
	IdempotencyKey string         `json:"idempotencyKey"`
	Audience       string         `json:"audience"`
	Arguments      map[string]any `json:"arguments"`
}

func (input *businessFilePrepareInput) UnmarshalJSON(raw []byte) error {
	type plain businessFilePrepareInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode((*plain)(input)); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("One exact file selection is required")
	}
	return nil
}

type businessFileID struct {
	FileID string `json:"fileId"`
}

func addBusinessFileTools(server *mcp.Server, backend *Backend) {
	uuid := map[string]any{"type": "string", "pattern": frameworkID.String()}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_prepare_business_file", Description: "Ask a person to select and privately upload a file for one exact reviewed business route. Supply all reviewed tool arguments except selectedFile. Choose actor for this signed-in person's assistant or deployment for that workload's SDK. The secure human handoff discloses the destination and audience; only the person can upload bytes. This does not approve or execute the business action. Keep the UUID key for identical preparation.",
		InputSchema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"routeId", "bindingSha256", "idempotencyKey", "audience", "arguments"}, "properties": map[string]any{"routeId": uuid, "bindingSha256": map[string]any{"type": "string", "pattern": businessBindingDigest.String()}, "idempotencyKey": uuid, "audience": map[string]any{"type": "string", "enum": []string{"actor", "deployment"}}, "arguments": map[string]any{"type": "object"}}}},
		func(ctx context.Context, _ *mcp.CallToolRequest, input businessFilePrepareInput) (*mcp.CallToolResult, map[string]any, error) {
			if !frameworkID.MatchString(input.RouteID) || !frameworkID.MatchString(input.IdempotencyKey) || !businessBindingDigest.MatchString(input.BindingSHA256) || input.Arguments == nil || (input.Audience != "actor" && input.Audience != "deployment") {
				return nil, nil, errors.New("Choose the exact reviewed action, audience and arguments")
			}
			if _, exists := input.Arguments["selectedFile"]; exists {
				return nil, nil, errors.New("A person selects the private file through the secure handoff")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.delegatedCall(ctx, token, "authrail_prepare_business_file", http.MethodPost, "/organisation/mcp/business-files", BusinessExecuteScope, input, identity)
			if err != nil {
				return nil, nil, err
			}
			safe, err := safeBusinessFile(data, true, "", identity.OrganisationID)
			return nil, safe, err
		})
	for _, operation := range []struct {
		name, method, suffix, description string
		read                              bool
	}{
		{"authrail_get_business_file", http.MethodGet, "", "Read original human file-selection status and exact reviewed metadata. A ready reference may be placed in selectedFile for the same route and unchanged arguments; use the original business request after dispatch. No bytes, upload capability or provider credentials are returned.", true},
		{"authrail_revoke_business_file", http.MethodPost, "/revoke", "Withdraw the original selected-file permission. This never undoes an attachment already sent to a provider and does not cancel or retry a business action.", false},
	} {
		mcp.AddTool(server, &mcp.Tool{Name: operation.name, Description: operation.description, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: operation.read}, InputSchema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"fileId"}, "properties": map[string]any{"fileId": uuid}}},
			func(ctx context.Context, _ *mcp.CallToolRequest, input businessFileID) (*mcp.CallToolResult, map[string]any, error) {
				if !frameworkID.MatchString(input.FileID) {
					return nil, nil, errors.New("Choose the original file-selection ID")
				}
				token, identity := requestIdentity(ctx)
				data, err := backend.delegatedCall(ctx, token, operation.name, operation.method, "/organisation/mcp/business-files/"+input.FileID+operation.suffix, BusinessExecuteScope, nil, identity)
				if err != nil {
					return nil, nil, err
				}
				safe, err := safeBusinessFile(data, false, input.FileID, identity.OrganisationID)
				return nil, safe, err
			})
	}
}
func safeBusinessFile(data map[string]any, prepare bool, expectedID, organisationID string) (map[string]any, error) {
	allowed := map[string]bool{"fileId": true, "routeId": true, "status": true, "audience": true, "reference": true, "argumentsSha256": true, "expiresAt": true, "boundToAction": true}
	if prepare {
		allowed["handoff"] = true
	}
	for key := range data {
		if !allowed[key] {
			return nil, errBackend
		}
	}
	id, _ := data["fileId"].(string)
	route, _ := data["routeId"].(string)
	hash, _ := data["argumentsSha256"].(string)
	state, _ := data["status"].(string)
	audience, _ := data["audience"].(string)
	states := map[string]bool{"awaiting_selection": true, "uploading": true, "ready": true, "expired": true, "revoked": true, "cleaning": true}
	if !frameworkID.MatchString(id) || expectedID != "" && id != expectedID || !frameworkID.MatchString(route) || !businessBindingDigest.MatchString(hash) || !states[state] || (audience != "actor" && audience != "deployment") {
		return nil, errBackend
	}
	if _, ok := data["boundToAction"].(bool); !ok {
		return nil, errBackend
	}
	expiry, hasExpiry := data["expiresAt"]
	if !hasExpiry || state == "ready" && expiry == nil {
		return nil, errBackend
	}
	if expiry != nil {
		value, ok := expiry.(string)
		if !ok {
			return nil, errBackend
		}
		if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
			return nil, errBackend
		}
	}
	value, hasReference := data["reference"]
	if !hasReference {
		return nil, errBackend
	}
	if value != nil {
		ref, ok := value.(map[string]any)
		if !ok || len(ref) != 5 {
			return nil, errBackend
		}
		refID, _ := ref["id"].(string)
		filename, _ := ref["filename"].(string)
		mime, _ := ref["contentType"].(string)
		sha, _ := ref["contentSha256"].(string)
		size, ok := ref["sizeBytes"].(float64)
		if refID != id || filename == "" || len(filename) > 640 || strings.ContainsAny(filename, "/\\\r\n\x00") || len(mime) > 120 || !strings.Contains(mime, "/") || !businessBindingDigest.MatchString(sha) || !ok || size < 1 || size > 20*1024*1024 || size != float64(int64(size)) {
			return nil, errBackend
		}
	} else if state == "ready" || state == "uploading" || state == "expired" || state == "cleaning" {
		return nil, errBackend
	}
	if prepare {
		handoff, ok := data["handoff"].(map[string]any)
		if !ok || len(handoff) != 2 || handoff["requiresHuman"] != true {
			return nil, errBackend
		}
		raw, ok := handoff["url"].(string)
		if !ok {
			return nil, errBackend
		}
		u, err := url.Parse(raw)
		if err != nil || u.User != nil || u.Fragment != "" || u.Path != "/runtime/files/"+id || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))) || len(u.Query()) != 1 || u.Query().Get("organisationId") != organisationID {
			return nil, errBackend
		}
	}
	return data, nil
}
