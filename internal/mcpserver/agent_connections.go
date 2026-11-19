package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func addAgentConnectionTools(server *mcp.Server, backend *Backend) {
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_agent_connections", Description: "List the connected workspace's A2A agent connections and saved review status. This reads saved records; it does not contact a server, grant tool access or reveal credentials. Continue using pageInfo.nextCursor.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input connectionListInput) (*mcp.CallToolResult, map[string]any, error) {
			if input.Limit < 0 || input.Limit > 10 || len(input.Cursor) > 500 {
				return nil, nil, errors.New("Use a page size from 1 to 10 and the previous Custom connections page's cursor")
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
			data, err := backend.delegatedRead(ctx, token, "authrail_list_agent_connections", "/organisation/mcp/connections/custom-a2a?"+query.Encode(), ConnectionsScope, identity)
			return nil, data, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_agent_connection", Description: "Read a A2A agent connection's saved status and reviewed tool summary. Account access and tool review are separate from permission to execute an action; credentials are never returned.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input connectionStatusInput) (*mcp.CallToolResult, map[string]any, error) {
			if !frameworkID.MatchString(input.ConnectionID) {
				return nil, nil, errors.New("Use the connectionId returned by authrail_list_agent_connections")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.delegatedRead(ctx, token, "authrail_get_agent_connection", "/organisation/mcp/connections/custom-a2a/"+input.ConnectionID, ConnectionsScope, identity)
			return nil, data, err
		})
}

func addAgentConnectionManagementTools(server *mcp.Server, backend *Backend) {
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_manage_agent_connection", Description: "Open a secure AuthRail page to connect a A2A agent server, review its skills, refresh its saved catalogue or disconnect it. The signed-in person must complete the action. This tool does not contact the server, publish tools or change account access. Never submit server addresses, credentials, passwords or tokens.", InputSchema: map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"action"},
		"properties": map[string]any{
			"action":       map[string]any{"type": "string", "enum": []string{"connect", "review", "refresh", "disconnect"}},
			"connectionId": map[string]any{"type": "string", "minLength": 36, "maxLength": 36, "description": "Required for review, refresh or disconnect; omit for connect"},
		},
	}}, func(ctx context.Context, _ *mcp.CallToolRequest, input customConnectionHandoffInput) (*mcp.CallToolResult, map[string]any, error) {
		switch input.Action {
		case "connect":
			if input.ConnectionID != "" {
				return nil, nil, errors.New("Omit the existing connection when connecting a new A2A agent server")
			}
		case "review", "refresh", "disconnect":
			if !frameworkID.MatchString(input.ConnectionID) {
				return nil, nil, errors.New("Choose an existing A2A agent connection")
			}
		default:
			return nil, nil, errors.New("Choose connect, review, refresh or disconnect")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedCall(ctx, token, "authrail_manage_agent_connection", http.MethodPost, "/organisation/mcp/connections/custom-a2a/handoff", ConnectionsManageScope, input, identity)
		return nil, data, err
	})
}
