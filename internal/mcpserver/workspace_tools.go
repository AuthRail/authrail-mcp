package mcpserver

import (
	"context"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
	"strconv"
)

const OrganisationReadScope = "organisation.read"
const OrganisationManageScope = "organisation.manage"
const AccountReadScope = "account.read"
const AccountManageScope = "account.manage"
const NotificationsReadScope = "notifications.read"
const NotificationsManageScope = "notifications.manage"

type workspacePageInput struct {
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque nextCursor from this exact tool/account/workspace/filter"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Page size 1 to 50; default 20"`
}
type workspaceRecordInput struct {
	RecordID string `json:"recordId" jsonschema:"Exact record UUID returned by this domain's list"`
}
type delegatedToolInput[I any] struct {
	name, description, scope, method string
	readonly                         bool
	prepare                          func(I) (string, any, error)
}

// Static registrations own every method/path and typed input; the model cannot select an API endpoint.
func addWorkspaceTool[I any](server *mcp.Server, backend *Backend, spec delegatedToolInput[I]) {
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: spec.readonly, IdempotentHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: spec.name, Description: spec.description, Annotations: annotations}, func(ctx context.Context, _ *mcp.CallToolRequest, input I) (*mcp.CallToolResult, map[string]any, error) {
		path, body, err := spec.prepare(input)
		if err != nil {
			return nil, nil, err
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedCall(ctx, token, spec.name, spec.method, path, spec.scope, body, identity)
		return nil, data, err
	})
}
func workspacePage(query url.Values, input workspacePageInput) (string, error) {
	if input.Limit < 0 || input.Limit > 50 || len(input.Cursor) > 2000 {
		return "", errors.New("Use a valid opaque cursor and page size from 1 to 50")
	}
	if input.Cursor != "" {
		query.Set("cursor", input.Cursor)
	}
	if input.Limit > 0 {
		query.Set("limit", strconv.Itoa(input.Limit))
	}
	return query.Encode(), nil
}
func addWorkspaceRead(server *mcp.Server, backend *Backend, name, description, scope, path string) {
	addWorkspaceTool(server, backend, delegatedToolInput[noArguments]{name: name, description: description, scope: scope, method: http.MethodGet, readonly: true, prepare: func(noArguments) (string, any, error) { return path, nil, nil }})
}
