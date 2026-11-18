package mcpserver

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BusinessToolsReadScope permits reviewed tool discovery, never execution or account management.
const BusinessToolsReadScope = "business.tools.read"

type availableToolListInput struct {
	Limit  *int   `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}
type availableToolInput struct {
	RouteID string `json:"routeId"`
}

func addBusinessTools(server *mcp.Server, backend *Backend) {
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_available_tools", Description: "List the connected workspace's currently reviewed business tools. Follow pageInfo.nextCursor for more. Discovery does not grant permission to execute a tool.", Annotations: annotations,
		InputSchema: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
			"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
			"cursor": map[string]any{"type": "string", "pattern": frameworkID.String()},
		}},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input availableToolListInput) (*mcp.CallToolResult, map[string]any, error) {
		limit := 10
		if input.Limit != nil {
			limit = *input.Limit
		}
		if limit < 1 || limit > 10 || input.Cursor != "" && !frameworkID.MatchString(input.Cursor) {
			return nil, nil, errors.New("Use a page size from 1 to 10 and the previous page's cursor")
		}
		query := url.Values{"limit": {strconv.Itoa(limit)}}
		if input.Cursor != "" {
			query.Set("cursor", input.Cursor)
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedRead(ctx, token, "authrail_list_available_tools", "/organisation/mcp/business-tools?"+query.Encode(), BusinessToolsReadScope, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_available_tool", Description: "Read the exact reviewed input and result contracts for an available business tool. This does not execute it or reveal private credentials or destinations.", Annotations: annotations,
		InputSchema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"routeId"}, "properties": map[string]any{
			"routeId": map[string]any{"type": "string", "pattern": frameworkID.String()},
		}},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input availableToolInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RouteID) {
			return nil, nil, errors.New("Choose the routeId returned by authrail_list_available_tools")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedRead(ctx, token, "authrail_get_available_tool", "/organisation/mcp/business-tools/"+input.RouteID, BusinessToolsReadScope, identity)
		return nil, data, err
	})
}
