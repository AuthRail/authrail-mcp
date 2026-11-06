package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type dataSourceListInput struct {
	Limit   int    `json:"limit,omitempty" jsonschema:"Page size 1 to 50; default 30"`
	AfterID string `json:"afterId,omitempty" jsonschema:"Last source ID from the previous page"`
}
type saveDataRailInput struct {
	RailID    string         `json:"railId" jsonschema:"Exact Data Rail ID"`
	VersionID string         `json:"versionId" jsonschema:"Exact Data Rail draft version ID"`
	Revision  int            `json:"revision" jsonschema:"Expected saved draft revision; reconcile after a conflict"`
	Name      string         `json:"name" jsonschema:"Business-readable Data Rail name, 1–180 characters"`
	Content   map[string]any `json:"content" jsonschema:"Complete Data Rail example content for its exact source: trusted, stale, unavailable and untrusted scenarios. No credentials, raw provider bodies or model configuration"`
}

type dataSourceInput struct {
	SourceID string `json:"sourceId" jsonschema:"Exact information-source ID in your workspace"`
}
type dataSourceVersionInput struct {
	SourceID  string `json:"sourceId" jsonschema:"Exact information-source ID"`
	VersionID string `json:"versionId" jsonschema:"Exact source-version ID; review status is returned by AuthRail"`
}
type createDataSourceInput struct {
	IdempotencyKey string         `json:"idempotencyKey" jsonschema:"UUID reused only for identical source creation retries"`
	Key            string         `json:"key" jsonschema:"Stable source key, 3–160 characters; starts with a letter and uses letters, digits, underscore, dot or hyphen"`
	Name           string         `json:"name" jsonschema:"Business-readable information source name, 1–180 characters"`
	Contract       map[string]any `json:"contract" jsonschema:"Complete source contract prepared from supported options: read lookup, account/resource binding, typed value, freshness and failure handling. No credentials, model settings or raw customer data"`
}
type saveDataSourceInput struct {
	SourceID  string         `json:"sourceId" jsonschema:"Exact source ID"`
	VersionID string         `json:"versionId" jsonschema:"Exact draft source-version ID"`
	Revision  int            `json:"revision" jsonschema:"Expected saved revision; re-read and reconcile after a conflict"`
	Name      string         `json:"name" jsonschema:"Business-readable source name, 1–180 characters"`
	Contract  map[string]any `json:"contract" jsonschema:"Complete reviewed source contract, without secrets or model configuration"`
}
type createDataRailInput struct {
	IdempotencyKey         string `json:"idempotencyKey" jsonschema:"UUID reused only for identical Rail creation retries"`
	Name                   string `json:"name" jsonschema:"Business-readable Data Rail name, 1–180 characters"`
	OperationVersionID     string `json:"operationVersionId" jsonschema:"Exact reviewed action contract that needs this fact"`
	SourceContextVersionID string `json:"sourceContextVersionId" jsonschema:"Exact human-reviewed source version, not a source parent ID or draft"`
}

var dataSourceKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{2,159}$`)

func dataDraftName(name string) bool { return strings.TrimSpace(name) != "" && len(name) <= 180 }

// Data preparation uses ordinary versioned source/Rail APIs; it does not execute a lookup.
func addDataDraftTools(server *mcp.Server, backend *Backend) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_data_source_options", Description: "Start Data Rail setup with the business value your rule needs, such as an invoice balance or customer consent. Read source options and examples, credential ownership, account and trusted-resource binding, value type, freshness and failure behaviour before preparing a source. OpenAPI and JSON Schema imports only describe candidate lookups or fields; they do not create live connections. AuthRail management MCP is separate from a customer MCP read-tool source. Credentials stay in the protected application; never supply them here.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArguments) (*mcp.CallToolResult, map[string]any, error) {
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_get_data_source_options", http.MethodGet, "/organisation/mcp/rails/data-source-options", nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_data_sources", Description: "Find information sources in your workspace. Read the selected source and its exact version before using it. A saved source definition does not establish a live reader, credential binding or trusted fact.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input dataSourceListInput) (*mcp.CallToolResult, map[string]any, error) {
		if input.Limit < 0 || input.Limit > 50 || input.AfterID != "" && !frameworkID.MatchString(input.AfterID) {
			return nil, nil, errors.New("Use a page size from 1 to 50 and the last source ID")
		}
		limit := input.Limit
		if limit == 0 {
			limit = 30
		}
		query := url.Values{"limit": {strconv.Itoa(limit)}}
		if input.AfterID != "" {
			query.Set("afterId", input.AfterID)
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_list_data_sources", http.MethodGet, "/organisation/mcp/rails/data-sources?"+query.Encode(), nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_data_source", Description: "Read one information source, its versions and review status before drafting changes or selecting it for a Data Rail. Source review is separate from testing a real lookup.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input dataSourceInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.SourceID) {
			return nil, nil, errors.New("Use the exact information-source ID")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_get_data_source", http.MethodGet, "/organisation/mcp/rails/data-sources/"+input.SourceID, nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_data_source_version", Description: "Inspect an exact source version and revision: read lookup, account/resource, returned value, freshness and failure behaviour. Select a human-reviewed version for a Data Rail. Re-read after a save conflict rather than overwriting another person's change.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input dataSourceVersionInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.SourceID) || !frameworkID.MatchString(input.VersionID) {
			return nil, nil, errors.New("Use exact source and version IDs")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_get_data_source_version", http.MethodGet, "/organisation/mcp/rails/data-sources/"+input.SourceID+"/versions/"+input.VersionID, nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_create_data_source_draft", Description: "Prepare an unpublished information-source definition after reading source options. Bind the read-only lookup, exact account/resource, typed value, freshness and unavailable behaviour. The application binds credentials privately once; a collection Connection does not grant Runtime access. Reuse the UUID only for identical retries. A person must review the source in AuthRail; this tool does not connect or call the provider."}, func(ctx context.Context, _ *mcp.CallToolRequest, input createDataSourceInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.IdempotencyKey) || !dataSourceKey.MatchString(input.Key) || !dataDraftName(input.Name) || input.Contract == nil {
			return nil, nil, errors.New("Supply a UUID creation key, stable source key, name and complete source contract without credentials")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_create_data_source_draft", http.MethodPost, "/organisation/mcp/rails/data-source-drafts", input, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_save_data_source_draft", Description: "Save an exact draft source revision with its complete account-bound read contract. Preserve freshness, timeout and failure rules. After a conflict, fetch that version and reconcile changes. This does not review the source, install a reader or connect credentials."}, func(ctx context.Context, _ *mcp.CallToolRequest, input saveDataSourceInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.SourceID) || !frameworkID.MatchString(input.VersionID) || input.Revision < 1 || !dataDraftName(input.Name) || input.Contract == nil {
			return nil, nil, errors.New("Supply exact source draft IDs, expected revision, name and complete contract")
		}
		token, identity := requestIdentity(ctx)
		body := map[string]any{"revision": input.Revision, "name": input.Name, "contract": input.Contract}
		data, err := backend.runtimeDraft(ctx, token, "authrail_save_data_source_draft", http.MethodPatch, "/organisation/mcp/rails/data-sources/"+input.SourceID+"/drafts/"+input.VersionID, body, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_create_data_rail_draft", Description: "Create a Data Rail draft joining one exact reviewed action and one exact human-reviewed source version. A Data Rail supplies a trusted fact; a Policy Rail decides permission. Reuse the UUID for identical retries. This does not publish, perform the lookup or run the business action."}, func(ctx context.Context, _ *mcp.CallToolRequest, input createDataRailInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.IdempotencyKey) || !frameworkID.MatchString(input.OperationVersionID) || !frameworkID.MatchString(input.SourceContextVersionID) || !dataDraftName(input.Name) {
			return nil, nil, errors.New("Supply a UUID creation key, Rail name, reviewed action version and reviewed source version")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_create_data_rail_draft", http.MethodPost, "/organisation/mcp/rails/data-drafts", input, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_save_data_rail_draft", Description: "Save an exact Data Rail draft revision and its examples for trusted, stale, unavailable and untrusted answers. A failed read must not become zero or false. Re-read and reconcile a conflicting draft; saving does not publish or establish a live connection."}, func(ctx context.Context, _ *mcp.CallToolRequest, input saveDataRailInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RailID) || !frameworkID.MatchString(input.VersionID) || input.Revision < 1 || !dataDraftName(input.Name) || input.Content == nil {
			return nil, nil, errors.New("Supply exact Rail draft IDs, expected revision, name and complete example content")
		}
		token, identity := requestIdentity(ctx)
		body := map[string]any{"revision": input.Revision, "name": input.Name, "content": input.Content}
		data, err := backend.runtimeDraft(ctx, token, "authrail_save_data_rail_draft", http.MethodPatch, "/organisation/mcp/rails/"+input.RailID+"/data-drafts/"+input.VersionID, body, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_test_data_rail_draft", Description: "Test saved examples for an exact Data Rail revision, including wrong account/resource, stale and unavailable answers. These examples do not call the live source or prove reader connectivity. A person must review publication in AuthRail; then inspect a real request's fact status after application binding."}, func(ctx context.Context, _ *mcp.CallToolRequest, input testRailInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RailID) || !frameworkID.MatchString(input.VersionID) || input.Revision < 1 {
			return nil, nil, errors.New("Supply exact Rail draft IDs and expected saved revision")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_test_data_rail_draft", http.MethodPost, "/organisation/mcp/rails/"+input.RailID+"/data-drafts/"+input.VersionID+"/test", map[string]int{"revision": input.Revision}, identity)
		return nil, data, err
	})
}
