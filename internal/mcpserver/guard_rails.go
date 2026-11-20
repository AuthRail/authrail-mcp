package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type guardRailIDInput struct {
	RailID string `json:"railId" jsonschema:"Guard Rail ID returned by AuthRail"`
}
type guardRailListInput struct {
	AfterID string `json:"afterId,omitempty" jsonschema:"Last Guard Rail ID from the previous bounded page"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Page size 1 to 50; default 30"`
}
type guardRailHistoryInput struct {
	RailID              string `json:"railId" jsonschema:"Guard Rail ID"`
	BeforeVersionNumber int    `json:"beforeVersionNumber,omitempty" jsonschema:"Last version number from the prior page; return older versions"`
	Limit               int    `json:"limit,omitempty" jsonschema:"Page size 1 to 50; default 30"`
}
type continueGuardRailInput struct {
	RailID             string `json:"railId" jsonschema:"Exact Guard Rail ID"`
	OperationVersionID string `json:"operationVersionId" jsonschema:"Exact reviewed action version"`
	BasedOnVersionID   string `json:"basedOnVersionId,omitempty" jsonschema:"Optional exact published version to copy only when the action is unchanged"`
}

func addGuardRailTools(server *mcp.Server, backend *Backend) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_guard_rails", Description: "Find existing Guard Rails and their current draft/published versions. Continue with the last Rail ID when the page is full. Read an exact version with authrail_get_rail_draft before editing. This does not change rules.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input guardRailListInput) (*mcp.CallToolResult, map[string]any, error) {
		if input.Limit < 0 || input.Limit > 50 || input.AfterID != "" && !frameworkID.MatchString(input.AfterID) {
			return nil, nil, errors.New("Use a page size from 1 to 50 and the last Guard Rail ID")
		}
		q := url.Values{}
		if input.Limit > 0 {
			q.Set("limit", strconv.Itoa(input.Limit))
		}
		if input.AfterID != "" {
			q.Set("afterId", input.AfterID)
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_list_guard_rails", http.MethodGet, "/organisation/mcp/guard-rails?"+q.Encode(), nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_guard_rail", Description: "Read one Guard Rail with its current draft and published versions using current workspace Rail and protection access. Select an exact version for examples, source provenance and editing.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input guardRailIDInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RailID) {
			return nil, nil, errors.New("Use the Guard Rail ID returned by AuthRail")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_get_guard_rail", http.MethodGet, "/organisation/mcp/guard-rails/"+input.RailID, nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_guard_rail_history", Description: "Read retained published and superseded Guard Rail versions, newest first. Follow the last version number for older history. Use authrail_get_rail_draft with an exact version ID for its unchanged rules and source provenance.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input guardRailHistoryInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RailID) || input.Limit < 0 || input.Limit > 50 || input.BeforeVersionNumber < 0 {
			return nil, nil, errors.New("Use an exact Guard Rail ID and bounded version-history page")
		}
		q := url.Values{}
		if input.Limit > 0 {
			q.Set("limit", strconv.Itoa(input.Limit))
		}
		if input.BeforeVersionNumber > 0 {
			q.Set("beforeVersionNumber", strconv.Itoa(input.BeforeVersionNumber))
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_get_guard_rail_history", http.MethodGet, "/organisation/mcp/guard-rails/"+input.RailID+"/history?"+q.Encode(), nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_continue_guard_rail_draft", Description: "Start an unpublished draft for a Guard Rail under current access. If another draft already exists, read that draft and reconcile it; do not repeat creation blindly. Copy an exact published version only when its action sources are unchanged. Changed sources require new rules and examples. Re-read the returned draft before saving; this never changes a published version."}, func(ctx context.Context, _ *mcp.CallToolRequest, input continueGuardRailInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RailID) || !frameworkID.MatchString(input.OperationVersionID) || input.BasedOnVersionID != "" && !frameworkID.MatchString(input.BasedOnVersionID) {
			return nil, nil, errors.New("Use the exact Guard Rail and source version IDs")
		}
		body := map[string]string{"operationVersionId": input.OperationVersionID}
		if input.BasedOnVersionID != "" {
			body["basedOnVersionId"] = input.BasedOnVersionID
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_continue_guard_rail_draft", http.MethodPost, "/organisation/mcp/guard-rails/"+input.RailID+"/drafts", body, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_review_guard_rail", Description: "Open exact human review of one saved Guard Rail draft revision. The signed-in person must review conditions, processing, permitted contextual material and example results and explicitly publish in AuthRail. This tool never records a human decision, changes a rule or activates a deployment."}, func(ctx context.Context, _ *mcp.CallToolRequest, input testRailInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RailID) || !frameworkID.MatchString(input.VersionID) || input.Revision < 1 {
			return nil, nil, errors.New("Read the exact saved draft revision before requesting human review")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_review_guard_rail", http.MethodPost, "/organisation/mcp/guard-rails/"+input.RailID+"/versions/"+input.VersionID+"/review", map[string]int{"revision": input.Revision}, identity)
		return nil, data, err
	})
}
