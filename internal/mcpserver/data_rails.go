package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type dataRailIDInput struct {
	RailID string `json:"railId" jsonschema:"Exact Data Rail ID"`
}
type dataRailListInput struct {
	AfterID string `json:"afterId,omitempty" jsonschema:"Last Rail ID from previous page"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Page size 1 to 50; default 30"`
}
type dataRailHistoryInput struct {
	RailID              string `json:"railId" jsonschema:"Exact Data Rail ID"`
	BeforeVersionNumber int    `json:"beforeVersionNumber,omitempty" jsonschema:"Last version number from previous page"`
	Limit               int    `json:"limit,omitempty" jsonschema:"Page size 1 to 50; default 30"`
}
type continueDataRailInput struct {
	RailID                 string `json:"railId" jsonschema:"Exact Data Rail ID"`
	OperationVersionID     string `json:"operationVersionId" jsonschema:"Exact reviewed action version"`
	SourceContextVersionID string `json:"sourceContextVersionId" jsonschema:"Exact reviewed source version"`
	BasedOnVersionID       string `json:"basedOnVersionId,omitempty" jsonschema:"Published version to copy only if action and source are unchanged"`
}
type dataSourceHistoryInput struct {
	SourceID            string `json:"sourceId" jsonschema:"Exact information source ID"`
	BeforeVersionNumber int    `json:"beforeVersionNumber,omitempty" jsonschema:"Last version number from previous page"`
	Limit               int    `json:"limit,omitempty" jsonschema:"Page size 1 to 50; default 30"`
}
type continueDataSourceInput struct {
	SourceID         string `json:"sourceId" jsonschema:"Exact information source ID"`
	BasedOnVersionID string `json:"basedOnVersionId" jsonschema:"Exact reviewed source version to copy"`
}
type reviewDataSourceInput struct {
	SourceID  string `json:"sourceId" jsonschema:"Exact information source ID"`
	VersionID string `json:"versionId" jsonschema:"Exact saved draft source version"`
	Revision  int    `json:"revision" jsonschema:"Expected saved source revision"`
}

func addDataRailTools(server *mcp.Server, backend *Backend) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_data_rails", Description: "Find existing Data Rails and their current draft/published versions. Continue with the last Rail ID when the page is full. Read an exact version with authrail_get_rail_draft before editing. This does not change rules.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input dataRailListInput) (*mcp.CallToolResult, map[string]any, error) {
		if input.Limit < 0 || input.Limit > 50 || input.AfterID != "" && !frameworkID.MatchString(input.AfterID) {
			return nil, nil, errors.New("Use a page size from 1 to 50 and the last Data Rail ID")
		}
		q := url.Values{}
		if input.Limit > 0 {
			q.Set("limit", strconv.Itoa(input.Limit))
		}
		if input.AfterID != "" {
			q.Set("afterId", input.AfterID)
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_list_data_rails", http.MethodGet, "/organisation/mcp/data-rails?"+q.Encode(), nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_data_rail", Description: "Read one Data Rail with its current draft and published versions using current workspace Rail access. Select an exact version for examples, source provenance and editing.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input dataRailIDInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RailID) {
			return nil, nil, errors.New("Use the Data Rail ID returned by AuthRail")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_get_data_rail", http.MethodGet, "/organisation/mcp/data-rails/"+input.RailID, nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_data_rail_history", Description: "Read retained published and superseded Data Rail versions, newest first. Follow the last version number for older history. Use authrail_get_rail_draft with an exact version ID for its unchanged examples and source provenance.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input dataRailHistoryInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RailID) || input.Limit < 0 || input.Limit > 50 || input.BeforeVersionNumber < 0 {
			return nil, nil, errors.New("Use an exact Data Rail ID and bounded version-history page")
		}
		q := url.Values{}
		if input.Limit > 0 {
			q.Set("limit", strconv.Itoa(input.Limit))
		}
		if input.BeforeVersionNumber > 0 {
			q.Set("beforeVersionNumber", strconv.Itoa(input.BeforeVersionNumber))
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_get_data_rail_history", http.MethodGet, "/organisation/mcp/data-rails/"+input.RailID+"/history?"+q.Encode(), nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_continue_data_rail_draft", Description: "Start the next unpublished Data Rail draft under current access. If a draft already exists, read the Rail and continue that saved draft. Copy an exact published version only when its information source and action are unchanged. Changed sources require new examples. Re-read the returned draft before saving; this never changes a published version."}, func(ctx context.Context, _ *mcp.CallToolRequest, input continueDataRailInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RailID) || !frameworkID.MatchString(input.OperationVersionID) || !frameworkID.MatchString(input.SourceContextVersionID) || input.BasedOnVersionID != "" && !frameworkID.MatchString(input.BasedOnVersionID) {
			return nil, nil, errors.New("Use the exact Data Rail and source version IDs")
		}
		body := map[string]string{"operationVersionId": input.OperationVersionID, "sourceContextVersionId": input.SourceContextVersionID}
		if input.BasedOnVersionID != "" {
			body["basedOnVersionId"] = input.BasedOnVersionID
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_continue_data_rail_draft", http.MethodPost, "/organisation/mcp/data-rails/"+input.RailID+"/drafts", body, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_review_data_rail", Description: "Open exact human review of one saved Data Rail draft revision. The signed-in person must review conditions, source account, resource, value, freshness and example results and explicitly publish in AuthRail. This tool never records a human decision, changes a rule or activates a deployment."}, func(ctx context.Context, _ *mcp.CallToolRequest, input testRailInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RailID) || !frameworkID.MatchString(input.VersionID) || input.Revision < 1 {
			return nil, nil, errors.New("Read the exact saved draft revision before requesting human review")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_review_data_rail", http.MethodPost, "/organisation/mcp/data-rails/"+input.RailID+"/versions/"+input.VersionID+"/review", map[string]int{"revision": input.Revision}, identity)
		return nil, data, err
	})

	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_data_source_history", Description: "Read retained reviewed information-source versions, newest first. Follow the last version number for older history. Read an exact source version before selecting it; this does not call a lookup.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input dataSourceHistoryInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.SourceID) || input.Limit < 0 || input.Limit > 50 || input.BeforeVersionNumber < 0 {
			return nil, nil, errors.New("Use an exact source ID and bounded history page")
		}
		q := url.Values{}
		if input.Limit > 0 {
			q.Set("limit", strconv.Itoa(input.Limit))
		}
		if input.BeforeVersionNumber > 0 {
			q.Set("beforeVersionNumber", strconv.Itoa(input.BeforeVersionNumber))
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_get_data_source_history", http.MethodGet, "/organisation/mcp/data-rails/sources/"+input.SourceID+"/history?"+q.Encode(), nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_continue_data_source_draft", Description: "Start an unpublished draft from one exact reviewed information-source version. If a draft already exists, read the source and continue that saved draft. The source account, bounded read, value, freshness and timeout are copied for a new review. This cannot modify reviewed versions or connect credentials."}, func(ctx context.Context, _ *mcp.CallToolRequest, input continueDataSourceInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.SourceID) || !frameworkID.MatchString(input.BasedOnVersionID) {
			return nil, nil, errors.New("Select exact source and reviewed version IDs")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_continue_data_source_draft", http.MethodPost, "/organisation/mcp/data-rails/sources/"+input.SourceID+"/drafts", map[string]string{"basedOnVersionId": input.BasedOnVersionID}, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_review_data_source", Description: "Open human review of the exact saved information-source draft revision. The signed-in person reviews account, read-only lookup, trusted record, typed value, freshness, timeout and unavailable behavior in AuthRail. The assistant cannot record that decision, connect credentials or prove a live reader."}, func(ctx context.Context, _ *mcp.CallToolRequest, input reviewDataSourceInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.SourceID) || !frameworkID.MatchString(input.VersionID) || input.Revision < 1 {
			return nil, nil, errors.New("Read the exact saved source draft and revision first")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_review_data_source", http.MethodPost, "/organisation/mcp/data-rails/sources/"+input.SourceID+"/versions/"+input.VersionID+"/review", map[string]int{"revision": input.Revision}, identity)
		return nil, data, err
	})
}
