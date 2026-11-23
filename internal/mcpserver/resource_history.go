package mcpserver

import (
	"context"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/url"
	"strconv"
)

type resourceHistoryInput struct {
	ConnectionID string `json:"connectionId" jsonschema:"Exact Connection ID"`
	ResourceID   string `json:"resourceId" jsonschema:"Exact discovered Resource ID"`
	Kind         string `json:"kind" jsonschema:"One of observations, decisions, evidence or tests"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Page size from 1 to 10; defaults to 10"`
	Cursor       string `json:"cursor,omitempty" jsonschema:"Previous cursor from this exact resource and history kind"`
}

func addResourceHistoryTool(server *mcp.Server, backend *Backend) {
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_resource_history", Description: "Read one page of a discovered Resource's observation metadata, retained monitoring decisions, Evidence references or Test results. Requires separate compliance verification read consent and current Resource permission. Raw observation payloads and credentials are withheld. Continue with pageInfo.nextCursor for the same resource and kind; collection history does not grant Runtime authority.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, _ *mcp.CallToolRequest, input resourceHistoryInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.ConnectionID) || !frameworkID.MatchString(input.ResourceID) || input.Limit < 0 || input.Limit > 10 || len(input.Cursor) > 1600 {
			return nil, nil, errors.New("Choose exact Connection/Resource IDs, a page size from 1 to 10 and that history's previous cursor")
		}
		switch input.Kind {
		case "observations", "decisions", "evidence", "tests":
		default:
			return nil, nil, errors.New("Choose observations, decisions, evidence or tests")
		}
		limit := input.Limit
		if limit == 0 {
			limit = 10
		}
		query := url.Values{"kind": {input.Kind}, "limit": {strconv.Itoa(limit)}}
		if input.Cursor != "" {
			query.Set("cursor", input.Cursor)
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedRead(ctx, token, "authrail_get_resource_history", "/organisation/mcp/connections/"+input.ConnectionID+"/resources/"+input.ResourceID+"/history?"+query.Encode(), ComplianceVerificationReadScope, identity)
		return nil, data, err
	})
}
