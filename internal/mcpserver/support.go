package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const SupportScope = "support.reports"

type supportReportInput struct {
	IdempotencyKey    string `json:"idempotencyKey" jsonschema:"Stable UUID for this report; reuse unchanged on retry"`
	Title             string `json:"title" jsonschema:"Problem summary, 3 to 160 characters"`
	Description       string `json:"description" jsonschema:"What happened, 10 to 8000 characters; include only deliberately reviewed material, never secrets"`
	ExpectedBehavior  string `json:"expectedBehavior,omitempty" jsonschema:"Expected outcome, up to 4000 characters"`
	ReproductionSteps string `json:"reproductionSteps,omitempty" jsonschema:"Known reproduction steps, up to 4000 characters"`
	ActionID          string `json:"actionId,omitempty" jsonschema:"Optional deliberately selected Action UUID in this workspace; requires Activity read access"`
}
type supportReportListInput struct {
	Status string `json:"status,omitempty" jsonschema:"Optional open, in_progress, needs_information, resolved or closed"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque nextCursor from the previous report page"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Page size 1 to 50; default 20"`
}
type supportReportIDInput struct {
	ReportID string `json:"reportId" jsonschema:"Report UUID returned by AuthRail"`
}

func addSupportTools(server *mcp.Server, backend *Backend) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_submit_problem_report", Description: "Report a problem to AuthRail support using explicitly reviewed material. AuthRail records the authenticated assistant connection and reporter; do not claim to be another person. Reuse the same submission UUID and unchanged details after an uncertain response. No credentials, raw provider payloads, full conversations or files are attached automatically.", Annotations: &mcp.ToolAnnotations{IdempotentHint: true}}, func(ctx context.Context, _ *mcp.CallToolRequest, input supportReportInput) (*mcp.CallToolResult, map[string]any, error) {
		input.Title = strings.TrimSpace(input.Title)
		input.Description = strings.TrimSpace(input.Description)
		if !frameworkID.MatchString(input.IdempotencyKey) || input.ActionID != "" && !frameworkID.MatchString(input.ActionID) || utf8.RuneCountInString(input.Title) < 3 || utf8.RuneCountInString(input.Title) > 160 || utf8.RuneCountInString(input.Description) < 10 || utf8.RuneCountInString(input.Description) > 8000 || utf8.RuneCountInString(input.ExpectedBehavior) > 4000 || utf8.RuneCountInString(input.ReproductionSteps) > 4000 {
			return nil, nil, errors.New("Supply a stable UUID, bounded report details and an optional exact Action reference")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedCall(ctx, token, "authrail_submit_problem_report", http.MethodPost, "/organisation/mcp/support/reports", SupportScope, input, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_my_problem_reports", Description: "List only the authenticated reporter’s submitted reports in this workspace. Internal support notes and other reporters’ records are excluded.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input supportReportListInput) (*mcp.CallToolResult, map[string]any, error) {
		if input.Limit < 0 || input.Limit > 50 || len(input.Cursor) > 500 || !validSupportStatus(input.Status) {
			return nil, nil, errors.New("Use a supported report status, cursor and page size up to 50")
		}
		query := url.Values{}
		if input.Status != "" {
			query.Set("status", input.Status)
		}
		if input.Cursor != "" {
			query.Set("cursor", input.Cursor)
		}
		if input.Limit > 0 {
			query.Set("limit", strconv.Itoa(input.Limit))
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedCall(ctx, token, "authrail_list_my_problem_reports", http.MethodGet, "/organisation/mcp/support/reports?"+query.Encode(), SupportScope, nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_my_problem_report", Description: "Follow your saved report’s current status and public support reply. This grants no Admin access or access to internal triage notes.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input supportReportIDInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.ReportID) {
			return nil, nil, errors.New("Select the exact saved report UUID")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedCall(ctx, token, "authrail_get_my_problem_report", http.MethodGet, "/organisation/mcp/support/reports/"+input.ReportID, SupportScope, nil, identity)
		return nil, data, err
	})
}
func validSupportStatus(status string) bool {
	switch status {
	case "", "open", "in_progress", "needs_information", "resolved", "closed":
		return true
	}
	return false
}
