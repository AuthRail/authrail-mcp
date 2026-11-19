package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type policyRailFactsInput struct {
	OperationVersionID string `json:"operationVersionId" jsonschema:"Exact reviewed action version"`
	AfterID            string `json:"afterId,omitempty" jsonschema:"Last Data Rail version ID from the previous page"`
	Limit              int    `json:"limit,omitempty" jsonschema:"Page size 1 to 50; default 30"`
}

type policyRailSourceVersionInput struct {
	PolicyID  string `json:"policyId" jsonschema:"Policy ID from the source list"`
	VersionID string `json:"versionId" jsonschema:"Exact published Policy version ID"`
}

type policyRailIDInput struct {
	RailID string `json:"railId" jsonschema:"Policy Rail ID returned by AuthRail"`
}
type policyRailListInput struct {
	AfterID string `json:"afterId,omitempty" jsonschema:"Last Policy Rail ID from the previous bounded page"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Page size 1 to 50; default 30"`
}
type policyRailHistoryInput struct {
	RailID              string `json:"railId" jsonschema:"Policy Rail ID"`
	BeforeVersionNumber int    `json:"beforeVersionNumber,omitempty" jsonschema:"Last version number from the prior page; return older versions"`
	Limit               int    `json:"limit,omitempty" jsonschema:"Page size 1 to 50; default 30"`
}
type policyRailSourceInput struct {
	Query  string `json:"query,omitempty" jsonschema:"Optional Policy title search, 2 to 120 characters"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque nextCursor from the previous source page"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Page size 1 to 25; default 20"`
}
type policyRailFactBinding struct {
	FactKey           string `json:"factKey" jsonschema:"Reviewed published Data Rail fact key"`
	DataRailVersionID string `json:"dataRailVersionId" jsonschema:"Exact published Data Rail version for this action"`
}
type policyRailProposalInput struct {
	OperationVersionID string                  `json:"operationVersionId" jsonschema:"Exact reviewed action version"`
	PolicyVersionID    string                  `json:"policyVersionId" jsonschema:"Exact published Policy version selected from authrail_list_rail_policy_sources"`
	Instructions       string                  `json:"instructions,omitempty" jsonschema:"Optional business-rule guidance, up to 1000 characters"`
	FactBindings       []policyRailFactBinding `json:"factBindings,omitempty" jsonschema:"Up to sixteen exact published Data Rail fact bindings"`
	ContextualAI       bool                    `json:"contextualAI,omitempty" jsonschema:"Prepare Rail-owned AuthRail Contextual AI for interpretation; review permitted disclosure before publication"`
}
type continuePolicyRailInput struct {
	RailID             string `json:"railId" jsonschema:"Existing Policy Rail ID"`
	OperationVersionID string `json:"operationVersionId" jsonschema:"Exact reviewed action version"`
	PolicyVersionID    string `json:"policyVersionId" jsonschema:"Exact published Policy version"`
	BasedOnVersionID   string `json:"basedOnVersionId,omitempty" jsonschema:"Exact published or superseded version to copy only when source versions are unchanged; omit for empty rules"`
}

func addPolicyRailTools(server *mcp.Server, backend *Backend) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_rail_policy_sources", Description: "Find published Policies for Policy Rail rules in the connected workspace. No Framework or Control setup is required. Choose the exact published version, read it with authrail_get_rail_policy_source, and follow pageInfo.nextCursor. This does not publish a Policy or Rail.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input policyRailSourceInput) (*mcp.CallToolResult, map[string]any, error) {
		if input.Limit < 0 || input.Limit > 25 || len(input.Cursor) > 500 || input.Query != "" && (len(strings.TrimSpace(input.Query)) < 2 || len(input.Query) > 120) {
			return nil, nil, errors.New("Use a source page size from 1 to 25, a bounded search and the saved cursor")
		}
		q := url.Values{}
		if input.Limit > 0 {
			q.Set("limit", strconv.Itoa(input.Limit))
		}
		if input.Query != "" {
			q.Set("query", input.Query)
		}
		if input.Cursor != "" {
			q.Set("cursor", input.Cursor)
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_list_rail_policy_sources", http.MethodGet, "/organisation/mcp/policy-rails/sources?"+q.Encode(), nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_rail_policy_source", Description: "Read one exact published Policy version selected for a Policy Rail. Current workspace permissions and source audience apply. Read its sections before translating them into draft conditions; this does not modify the Policy.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input policyRailSourceVersionInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.PolicyID) || !frameworkID.MatchString(input.VersionID) {
			return nil, nil, errors.New("Select the exact Policy and published version IDs")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_get_rail_policy_source", http.MethodGet, "/organisation/mcp/policy-rails/sources/"+input.PolicyID+"/versions/"+input.VersionID, nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_policy_rails", Description: "Find existing Policy Rails and their current draft/published versions. Continue with the last Rail ID when the page is full. Read an exact version with authrail_get_rail_draft before editing. This does not change rules.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input policyRailListInput) (*mcp.CallToolResult, map[string]any, error) {
		if input.Limit < 0 || input.Limit > 50 || input.AfterID != "" && !frameworkID.MatchString(input.AfterID) {
			return nil, nil, errors.New("Use a page size from 1 to 50 and the last Policy Rail ID")
		}
		q := url.Values{}
		if input.Limit > 0 {
			q.Set("limit", strconv.Itoa(input.Limit))
		}
		if input.AfterID != "" {
			q.Set("afterId", input.AfterID)
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_list_policy_rails", http.MethodGet, "/organisation/mcp/policy-rails?"+q.Encode(), nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_policy_rail", Description: "Read one Policy Rail with its current draft and published versions using current workspace Rail and Policy access. Select an exact version for examples, source provenance and editing.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input policyRailIDInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RailID) {
			return nil, nil, errors.New("Use the Policy Rail ID returned by AuthRail")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_get_policy_rail", http.MethodGet, "/organisation/mcp/policy-rails/"+input.RailID, nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_policy_rail_history", Description: "Read retained published and superseded Policy Rail versions, newest first. Follow the last version number for older history. Use authrail_get_rail_draft with an exact version ID for its unchanged rules and source provenance.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input policyRailHistoryInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RailID) || input.Limit < 0 || input.Limit > 50 || input.BeforeVersionNumber < 0 {
			return nil, nil, errors.New("Use an exact Policy Rail ID and bounded version-history page")
		}
		q := url.Values{}
		if input.Limit > 0 {
			q.Set("limit", strconv.Itoa(input.Limit))
		}
		if input.BeforeVersionNumber > 0 {
			q.Set("beforeVersionNumber", strconv.Itoa(input.BeforeVersionNumber))
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_get_policy_rail_history", http.MethodGet, "/organisation/mcp/policy-rails/"+input.RailID+"/history?"+q.Encode(), nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_policy_data_facts", Description: "Find permitted published Data Rail facts for one reviewed action. Bind only the exact fact key/version returned here when preparing Policy rules. Continue with the last fact version ID when the bounded page is full. This reads definitions; it does not query a customer account or execute an action.", Annotations: readOnly}, func(ctx context.Context, _ *mcp.CallToolRequest, input policyRailFactsInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.OperationVersionID) || input.Limit < 0 || input.Limit > 50 || input.AfterID != "" && !frameworkID.MatchString(input.AfterID) {
			return nil, nil, errors.New("Select an exact reviewed action version and bounded fact page")
		}
		q := url.Values{"operationVersionId": {input.OperationVersionID}}
		if input.Limit > 0 {
			q.Set("limit", strconv.Itoa(input.Limit))
		}
		if input.AfterID != "" {
			q.Set("afterId", input.AfterID)
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_list_policy_data_facts", http.MethodGet, "/organisation/mcp/policy-rails/data-facts?"+q.Encode(), nil, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_propose_policy_rail", Description: "Suggest unsaved business rules from an exact published Policy and reviewed action, with optional exact Data Rail facts. Inspect source excerpts, conditions and unresolved questions before saving a draft. Contextual AI is optional and AuthRail-operated. Suggestions cannot publish rules or execute an action."}, func(ctx context.Context, _ *mcp.CallToolRequest, input policyRailProposalInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.OperationVersionID) || !frameworkID.MatchString(input.PolicyVersionID) || len(input.Instructions) > 1000 || len(input.FactBindings) > 16 {
			return nil, nil, errors.New("Select exact source versions and bounded proposal guidance")
		}
		keys := map[string]bool{}
		versions := map[string]bool{}
		for _, binding := range input.FactBindings {
			if !policyRailFactKey.MatchString(binding.FactKey) || !frameworkID.MatchString(binding.DataRailVersionID) || keys[binding.FactKey] || versions[binding.DataRailVersionID] {
				return nil, nil, errors.New("Use distinct reviewed fact keys and Data Rail versions")
			}
			keys[binding.FactKey] = true
			versions[binding.DataRailVersionID] = true
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_propose_policy_rail", http.MethodPost, "/organisation/mcp/policy-rails/proposals", input, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_continue_policy_rail_draft", Description: "Start or recover the existing unpublished draft for a Policy Rail under current access. Copy an exact published version only when its Policy and action sources are unchanged. Changed sources require new rules and examples. Re-read the returned draft before saving; this never changes a published version."}, func(ctx context.Context, _ *mcp.CallToolRequest, input continuePolicyRailInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RailID) || !frameworkID.MatchString(input.OperationVersionID) || !frameworkID.MatchString(input.PolicyVersionID) || input.BasedOnVersionID != "" && !frameworkID.MatchString(input.BasedOnVersionID) {
			return nil, nil, errors.New("Use the exact Policy Rail and source version IDs")
		}
		body := map[string]string{"operationVersionId": input.OperationVersionID, "policyVersionId": input.PolicyVersionID}
		if input.BasedOnVersionID != "" {
			body["basedOnVersionId"] = input.BasedOnVersionID
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_continue_policy_rail_draft", http.MethodPost, "/organisation/mcp/policy-rails/"+input.RailID+"/drafts", body, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_review_policy_rail", Description: "Open exact human review of one saved Policy Rail draft revision. The signed-in person must review conditions, Policy excerpts, facts and example results and explicitly publish in AuthRail. This tool never records a human decision, changes a rule or activates a deployment."}, func(ctx context.Context, _ *mcp.CallToolRequest, input testRailInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RailID) || !frameworkID.MatchString(input.VersionID) || input.Revision < 1 {
			return nil, nil, errors.New("Read the exact saved draft revision before requesting human review")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.runtimeDraft(ctx, token, "authrail_review_policy_rail", http.MethodPost, "/organisation/mcp/policy-rails/"+input.RailID+"/versions/"+input.VersionID+"/review", map[string]int{"revision": input.Revision}, identity)
		return nil, data, err
	})
}

var policyRailFactKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,79}$`)
