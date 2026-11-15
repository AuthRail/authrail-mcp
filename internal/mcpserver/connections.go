package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ConnectionsManageScope = "connections.manage"

type connectionListInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"Page size from 1 to 10; defaults to 10"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque next cursor from the previous Connections page"`
}
type connectionStatusInput struct {
	ConnectionID string `json:"connectionId" jsonschema:"Connection ID returned by authrail_list_connections"`
}

type connectionHandoffInput struct {
	Action              string `json:"action" jsonschema:"One of connect, reconnect or disconnect; opens a secure page for the person to finish"`
	DefinitionVersionID string `json:"definitionVersionId,omitempty" jsonschema:"Exact provider definition version, required only when connecting a new account"`
	ConnectionID        string `json:"connectionId,omitempty" jsonschema:"Existing account connection, required only for reconnect or disconnect"`
}

func addConnectionManagementTools(server *mcp.Server, backend *Backend) {
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_connection_providers", Description: "Find available account providers and their exact setup definition versions before opening a secure connection page. This lists setup options; it does not grant account access or establish that business tools are available. Continue using pageInfo.nextCursor.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
		func(ctx context.Context, _ *mcp.CallToolRequest, input struct {
			Limit  int    `json:"limit,omitempty" jsonschema:"Page size from 1 to 10; defaults to 5"`
			Cursor string `json:"cursor,omitempty" jsonschema:"Opaque next cursor from the previous providers page"`
			Search string `json:"search,omitempty" jsonschema:"Provider or product name to find, at most 160 characters"`
		}) (*mcp.CallToolResult, map[string]any, error) {
			if input.Limit < 0 || input.Limit > 10 || len(input.Cursor) > 500 || len(input.Search) > 160 {
				return nil, nil, errors.New("Use a page size from 1 to 10, a provider name up to 160 characters and the previous providers page's cursor")
			}
			limit := input.Limit
			if limit == 0 {
				limit = 5
			}
			query := url.Values{"limit": {strconv.Itoa(limit)}}
			if input.Cursor != "" {
				query.Set("cursor", input.Cursor)
			}
			if input.Search != "" {
				query.Set("search", input.Search)
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.delegatedRead(ctx, token, "authrail_list_connection_providers", "/organisation/mcp/connections/providers?"+query.Encode(), ConnectionsManageScope, identity)
			return nil, data, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_manage_connection", Description: "Open a secure AuthRail page to connect, reconnect or disconnect a business account. Requires separate account-management consent and live workspace permission. The person must sign in and complete the action; this tool does not grant provider access, change credentials or disconnect an account. Never submit passwords, keys, access tokens or a provider URL.", InputSchema: map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"action"},
		"properties": map[string]any{
			"action":              map[string]any{"type": "string", "enum": []string{"connect", "reconnect", "disconnect"}},
			"definitionVersionId": map[string]any{"type": "string", "minLength": 36, "maxLength": 36, "description": "Required only for connect; the provider definition version"},
			"connectionId":        map[string]any{"type": "string", "minLength": 36, "maxLength": 36, "description": "Required only for reconnect or disconnect; the existing account connection"},
		},
	}},
		func(ctx context.Context, _ *mcp.CallToolRequest, input connectionHandoffInput) (*mcp.CallToolResult, map[string]any, error) {
			switch input.Action {
			case "connect":
				if !frameworkID.MatchString(input.DefinitionVersionID) || input.ConnectionID != "" {
					return nil, nil, errors.New("Choose a provider definition version to connect a new account")
				}
			case "reconnect", "disconnect":
				if !frameworkID.MatchString(input.ConnectionID) || input.DefinitionVersionID != "" {
					return nil, nil, errors.New("Choose the existing connection to reconnect or disconnect")
				}
			default:
				return nil, nil, errors.New("Choose connect, reconnect or disconnect")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.delegatedCall(ctx, token, "authrail_manage_connection", http.MethodPost, "/organisation/mcp/connections/handoff", ConnectionsManageScope, input, identity)
			return nil, data, err
		})
}

func addConnectionTools(server *mcp.Server, backend *Backend) {
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_connections", Description: "List the connected workspace’s provider Connections and saved health. This does not disclose credentials, contact a provider or start a sync. Continue using pageInfo.nextCursor.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input connectionListInput) (*mcp.CallToolResult, map[string]any, error) {
			if input.Limit < 0 || input.Limit > 10 || len(input.Cursor) > 500 {
				return nil, nil, errors.New("Use a page size from 1 to 10 and the previous Connections page’s cursor")
			}
			limit := input.Limit
			if limit == 0 {
				limit = 10
			}
			query := url.Values{"limit": {strconv.Itoa(limit)}}
			if input.Cursor != "" {
				query.Set("cursor", input.Cursor)
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.delegatedRead(ctx, token, "authrail_list_connections", "/organisation/mcp/connections?"+query.Encode(), ConnectionsScope, identity)
			return nil, data, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_connection_status", Description: "Inspect one provider Connection’s saved status and last successful collection time. Connection health is not proof that a compliance Test passed.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input connectionStatusInput) (*mcp.CallToolResult, map[string]any, error) {
			if !frameworkID.MatchString(input.ConnectionID) {
				return nil, nil, errors.New("Use the connectionId returned by authrail_list_connections")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.delegatedRead(ctx, token, "authrail_get_connection_status", "/organisation/mcp/connections/"+input.ConnectionID, ConnectionsScope, identity)
			return nil, data, err
		})
}
