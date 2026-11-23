package mcpserver

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const RuntimeReadScope = "runtime.read"

func addRuntimeRecordTools(server *mcp.Server, backend *Backend) {
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_runtime_action", Description: "Inspect a retained Action using the deployment and Action IDs from the protected connector. Requires separate Runtime read consent and your AI Inventory visibility. Returns bounded status metadata. Select historyKind certificates, receipts or approvals for a metadata page of up to ten rows, then follow its UUID cursor with the same historyKind. Without historyKind, returns the existing Action summary. Private statements, signatures, assertions, provider references and approval review material are withheld. Permission, application-reported execution and independently verified provider effects remain distinct; Observe does not prove enforcement.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}, InputSchema: map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"deploymentId", "actionId"}, "properties": map[string]any{
			"deploymentId":  map[string]any{"type": "string", "pattern": frameworkID.String()},
			"actionId":      map[string]any{"type": "string", "pattern": frameworkID.String()},
			"historyKind":   map[string]any{"type": "string", "enum": []string{"certificates", "receipts", "approvals"}},
			"historyCursor": map[string]any{"type": "string", "pattern": frameworkID.String()},
			"limit":         map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
		},
	}}, func(ctx context.Context, _ *mcp.CallToolRequest, input struct {
		DeploymentID  string  `json:"deploymentId"`
		ActionID      string  `json:"actionId"`
		HistoryKind   *string `json:"historyKind,omitempty"`
		HistoryCursor *string `json:"historyCursor,omitempty"`
		Limit         *int    `json:"limit,omitempty"`
	}) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.DeploymentID) || !frameworkID.MatchString(input.ActionID) {
			return nil, nil, errors.New("Use the exact deployment and Action IDs returned for your request")
		}
		path := "/organisation/mcp/runtime/deployments/" + input.DeploymentID + "/actions/" + input.ActionID
		if input.HistoryKind == nil {
			if input.HistoryCursor != nil || input.Limit != nil {
				return nil, nil, errors.New("Choose historyKind before using a history cursor or page size")
			}
		} else {
			kind := *input.HistoryKind
			if kind != "certificates" && kind != "receipts" && kind != "approvals" {
				return nil, nil, errors.New("Choose certificates, receipts or approvals for historyKind")
			}
			limit := 10
			if input.Limit != nil {
				limit = *input.Limit
			}
			if limit < 1 || limit > 10 || input.HistoryCursor != nil && !frameworkID.MatchString(*input.HistoryCursor) {
				return nil, nil, errors.New("Use a history page size from 1 to 10 and the previous page's UUID cursor")
			}
			query := url.Values{"kind": {kind}, "limit": {strconv.Itoa(limit)}}
			if input.HistoryCursor != nil {
				query.Set("cursor", *input.HistoryCursor)
			}
			path += "/history?" + query.Encode()
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedRead(ctx, token, "authrail_get_runtime_action", path, RuntimeReadScope, identity)
		return nil, data, err
	})
}
