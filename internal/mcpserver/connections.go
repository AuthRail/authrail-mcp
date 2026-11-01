package mcpserver

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type connectionListInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"Page size from 1 to 10; defaults to 10"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque next cursor from the previous Connections page"`
}
type connectionStatusInput struct {
	ConnectionID string `json:"connectionId" jsonschema:"Connection ID returned by authrail_list_connections"`
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
