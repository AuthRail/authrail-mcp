package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type createActionInput struct {
	Key            string         `json:"key" jsonschema:"Stable application action key, 3 to 160 characters; not a provider URL"`
	Name           string         `json:"name" jsonschema:"Action name, at most 180 characters"`
	Contract       map[string]any `json:"contract" jsonschema:"Complete untrusted candidate Operation contract: summary, inputSchema, resultSchema, availableFields, resourceIdentity, expectedEffects, requiredFacts, approvalSupported, idempotency, requiredCoverage. Schemas describe fields; they grant no live connection or permission"`
	IdempotencyKey string         `json:"idempotencyKey" jsonschema:"UUID reused only for identical creation retries"`
}
type saveActionInput struct {
	OperationID string         `json:"operationId"`
	VersionID   string         `json:"versionId"`
	Revision    int            `json:"revision" jsonschema:"Exact draft revision last read; a conflict requires reconciliation"`
	Name        string         `json:"name"`
	Contract    map[string]any `json:"contract" jsonschema:"Complete candidate Operation contract; importing MCP metadata does not confirm it"`
}

type actionVersionInput struct {
	OperationID string `json:"operationId"`
	VersionID   string `json:"versionId"`
}
type actionHistoryInput struct {
	OperationID string `json:"operationId"`
	Cursor      string `json:"cursor,omitempty" jsonschema:"Exact history cursor from the preceding page for this same action"`
	Limit       int    `json:"limit,omitempty" jsonschema:"Page size from 1 to 50; defaults to 25"`
}
type continueActionInput struct {
	OperationID      string `json:"operationId"`
	BasedOnVersionID string `json:"basedOnVersionId" jsonschema:"Exact confirmed predecessor version from this action"`
	Revision         int    `json:"revision" jsonschema:"Exact predecessor revision last read"`
	IdempotencyKey   string `json:"idempotencyKey" jsonschema:"UUID retained for identical continuation retries, including lost responses"`
}

func addActionDraftTools(server *mcp.Server, backend *Backend) {
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_connected_action_version", Description: "Read one exact retained connected action contract and its lifecycle. A draft includes a link for the person to review that exact revision. Superseded versions describe history and existing exact bindings; they cannot be selected as a new confirmed action. This does not perform the action.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, _ *mcp.CallToolRequest, input actionVersionInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.OperationID) || !frameworkID.MatchString(input.VersionID) {
			return nil, nil, errors.New("Supply exact action and version IDs")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_get_connected_action_version", http.MethodGet, "/organisation/mcp/rails/actions/"+input.OperationID+"/versions/"+input.VersionID, nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_connected_action_history", Description: "Read confirmed connected action version summaries, newest first. Follow the returned cursor with the same action. Read an exact version for its contract; history does not change Rails or active deployments.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, _ *mcp.CallToolRequest, input actionHistoryInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.OperationID) || input.Cursor != "" && !frameworkID.MatchString(input.Cursor) || input.Limit < 0 || input.Limit > 50 {
			return nil, nil, errors.New("Supply an action ID, its exact optional cursor and a page size up to 50")
		}
		limit := input.Limit
		if limit == 0 {
			limit = 25
		}
		query := url.Values{"limit": {strconv.Itoa(limit)}}
		if input.Cursor != "" {
			query.Set("cursor", input.Cursor)
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_get_connected_action_history", http.MethodGet, "/organisation/mcp/rails/actions/"+input.OperationID+"/versions?"+query.Encode(), nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_continue_connected_action_draft", Description: "Copy one exact confirmed action version into an unconfirmed successor draft under the same action identity. Retain the UUID retry key and predecessor revision for identical retries. A retry returns its original successor even if later reviewed; it never creates another draft automatically. Human review of the exact draft remains required. This does not replace a Rail, change an active deployment, grant approval or execute business work."}, func(ctx context.Context, _ *mcp.CallToolRequest, input continueActionInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.OperationID) || !frameworkID.MatchString(input.BasedOnVersionID) || !frameworkID.MatchString(input.IdempotencyKey) || input.Revision < 1 {
			return nil, nil, errors.New("Supply exact action/predecessor IDs, predecessor revision and UUID retry key")
		}
		token, identity := requestIdentity(ctx)
		body := map[string]any{"basedOnVersionId": input.BasedOnVersionID, "revision": input.Revision, "idempotencyKey": input.IdempotencyKey}
		data, err := backend.runtimeDraft(ctx, token, "authrail_continue_connected_action_draft", http.MethodPost, "/organisation/mcp/rails/actions/"+input.OperationID+"/drafts", body, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_protected_mcp_setup", Description: "Connect an existing customer MCP business-tool server through the customer-side protected connector. Read the exact local command flow, candidate action contract, human confirmation and saved approval responsibilities. Use authorised local tools for private configuration; this hosted tool cannot install software or access customer credentials. The connector uses the Go SDK internally, so the user does not wire an SDK manually.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArguments) (*mcp.CallToolResult, map[string]any, error) {
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_get_protected_mcp_setup", http.MethodGet, "/organisation/mcp/integrations/protected-mcp", nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_create_connected_action_draft", Description: "Prepare the description of one real business tool before creating its Rails. Review its inputs, normalised result, resource ownership, actual effects and idempotency. Use the returned human review link to confirm the exact version; this tool cannot confirm an action, publish a Rail, install a connector or perform the business operation."}, func(ctx context.Context, _ *mcp.CallToolRequest, input createActionInput) (*mcp.CallToolResult, map[string]any, error) {
		if len(strings.TrimSpace(input.Key)) < 3 || len(input.Key) > 160 || strings.TrimSpace(input.Name) == "" || len(input.Name) > 180 || input.Contract == nil || !frameworkID.MatchString(input.IdempotencyKey) {
			return nil, nil, errors.New("Supply an action name, stable key, candidate contract and UUID creation key")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_create_connected_action_draft", http.MethodPost, "/organisation/mcp/rails/action-drafts", input, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_save_connected_action_draft", Description: "Update one unconfirmed action using its expected revision. Re-read and reconcile a conflict. Changes require fresh human review of the exact version; the agent cannot mark the contract confirmed."}, func(ctx context.Context, _ *mcp.CallToolRequest, input saveActionInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.OperationID) || !frameworkID.MatchString(input.VersionID) || input.Revision < 1 || strings.TrimSpace(input.Name) == "" || len(input.Name) > 180 || input.Contract == nil {
			return nil, nil, errors.New("Supply exact action/version IDs, expected revision, name and complete contract")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_save_connected_action_draft", http.MethodPatch, "/organisation/mcp/rails/actions/"+input.OperationID+"/drafts/"+input.VersionID, map[string]any{"revision": input.Revision, "name": input.Name, "contract": input.Contract}, identity)
		return nil, data, err
	})
}
