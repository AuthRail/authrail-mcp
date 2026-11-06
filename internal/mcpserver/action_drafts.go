package mcpserver

import (
	"context"
	"errors"
	"net/http"
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

func addActionDraftTools(server *mcp.Server, backend *Backend) {
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
