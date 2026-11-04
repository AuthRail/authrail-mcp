package mcpserver

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type policyBasesInput struct {
	ControlID        string `json:"controlId" jsonschema:"Exact Control ID"`
	ControlVersionID string `json:"controlVersionId" jsonschema:"Exact Control version whose Policy references should be read"`
	Limit            int    `json:"limit,omitempty" jsonschema:"Page size 1 to 10; defaults to 10"`
	Cursor           string `json:"cursor,omitempty" jsonschema:"Opaque previous Policy-reference cursor"`
}
type policyVersionInput struct {
	PolicyID        string `json:"policyId" jsonschema:"Policy ID returned by the Control Policy references"`
	PolicyVersionID string `json:"policyVersionId" jsonschema:"Exact Policy version ID; do not substitute the latest draft"`
}
type testRunsInput struct {
	ControlID               string `json:"controlId" jsonschema:"Exact Control ID"`
	ControlVersionID        string `json:"controlVersionId" jsonschema:"Exact Control version ID"`
	ImplementationVersionID string `json:"implementationVersionId,omitempty" jsonschema:"Exact scoped Implementation owner when applicable"`
	DefinitionID            string `json:"definitionId" jsonschema:"Test definition version record ID returned as id by authrail_list_tests"`
	Limit                   int    `json:"limit,omitempty" jsonschema:"Page size 1 to 10; defaults to 10"`
	Cursor                  string `json:"cursor,omitempty" jsonschema:"Opaque previous run-history cursor"`
}
type evidenceHistoryInput struct {
	ControlID               string `json:"controlId" jsonschema:"Exact Control ID"`
	ControlVersionID        string `json:"controlVersionId" jsonschema:"Exact Control version ID"`
	ImplementationVersionID string `json:"implementationVersionId,omitempty" jsonschema:"Exact scoped Implementation owner when applicable"`
	RequirementID           string `json:"requirementId" jsonschema:"Exact requirement version record ID returned as id by authrail_list_evidence_requirements"`
	Limit                   int    `json:"limit,omitempty" jsonschema:"Page size 1 to 10; defaults to 10"`
	Cursor                  string `json:"cursor,omitempty" jsonschema:"Opaque previous record-history cursor"`
}
type evidenceDecisionsInput struct {
	ControlID               string `json:"controlId" jsonschema:"Exact Control ID"`
	ControlVersionID        string `json:"controlVersionId" jsonschema:"Exact Control version ID"`
	ImplementationVersionID string `json:"implementationVersionId,omitempty" jsonschema:"Exact scoped Implementation owner when applicable"`
	RequirementID           string `json:"requirementId" jsonschema:"Exact requirement version record ID"`
	RecordID                string `json:"recordId" jsonschema:"Evidence Record ID returned by authrail_list_evidence_records"`
	Limit                   int    `json:"limit,omitempty" jsonschema:"Page size 1 to 10; defaults to 10"`
	Cursor                  string `json:"cursor,omitempty" jsonschema:"Opaque previous decision-history cursor"`
}

func addHistoryTools(server *mcp.Server, backend *Backend) {
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_control_policy_bases", Description: "Read distinct exact Policy versions referenced by a visible Control version. Restricted means hidden references exist. Private sections and document bodies are excluded.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input policyBasesInput) (*mcp.CallToolResult, map[string]any, error) {
			query, err := historyQuery(input.Limit, input.Cursor, "")
			if err != nil {
				return nil, nil, err
			}
			if !frameworkID.MatchString(input.ControlID) || !frameworkID.MatchString(input.ControlVersionID) {
				return nil, nil, errors.New("Use exact Control and Control version IDs")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.governance(ctx, token, "authrail_list_control_policy_bases", controlOwnerPath(input.ControlID, input.ControlVersionID)+"/policy-bases?"+query, identity)
			return nil, data, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_policy_version", Description: "Read metadata for one exact permitted Policy version. This does not choose a newer draft or return document text.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input policyVersionInput) (*mcp.CallToolResult, map[string]any, error) {
			if !frameworkID.MatchString(input.PolicyID) || !frameworkID.MatchString(input.PolicyVersionID) {
				return nil, nil, errors.New("Use exact Policy and Policy version IDs")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.governance(ctx, token, "authrail_get_policy_version", "/organisation/mcp/policies/"+input.PolicyID+"/versions/"+input.PolicyVersionID, identity)
			return nil, data, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_test_runs", Description: "Read bounded recorded Test Run outcomes, periods and evaluator metadata for the exact Test version. Run inputs and explanations are excluded. A reported result is not independent provider verification.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input testRunsInput) (*mcp.CallToolResult, map[string]any, error) {
			query, err := historyQuery(input.Limit, input.Cursor, input.ImplementationVersionID)
			if err != nil {
				return nil, nil, err
			}
			if !frameworkID.MatchString(input.ControlID) || !frameworkID.MatchString(input.ControlVersionID) || !frameworkID.MatchString(input.DefinitionID) {
				return nil, nil, errors.New("Use the exact Control owner and Test version record id")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.governance(ctx, token, "authrail_list_test_runs", controlOwnerPath(input.ControlID, input.ControlVersionID)+"/tests/"+input.DefinitionID+"/runs?"+query, identity)
			return nil, data, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_evidence_records", Description: "Read Evidence Record periods, revisions and review states for an exact requirement. Attestations, file keys, source contents and download URLs are excluded. Review status alone does not prove a provider effect.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input evidenceHistoryInput) (*mcp.CallToolResult, map[string]any, error) {
			query, err := historyQuery(input.Limit, input.Cursor, input.ImplementationVersionID)
			if err != nil {
				return nil, nil, err
			}
			if !frameworkID.MatchString(input.ControlID) || !frameworkID.MatchString(input.ControlVersionID) || !frameworkID.MatchString(input.RequirementID) {
				return nil, nil, errors.New("Use the exact Control owner and requirement version record id")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.governance(ctx, token, "authrail_list_evidence_records", controlOwnerPath(input.ControlID, input.ControlVersionID)+"/evidence-requirements/"+input.RequirementID+"/records?"+query, identity)
			return nil, data, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_evidence_record_decisions", Description: "Read retained review decisions, reviewer references and timestamps for one exact Evidence Record. Private review text is excluded; no decision is submitted by this read.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input evidenceDecisionsInput) (*mcp.CallToolResult, map[string]any, error) {
			query, err := historyQuery(input.Limit, input.Cursor, input.ImplementationVersionID)
			if err != nil {
				return nil, nil, err
			}
			if !frameworkID.MatchString(input.ControlID) || !frameworkID.MatchString(input.ControlVersionID) || !frameworkID.MatchString(input.RequirementID) || !frameworkID.MatchString(input.RecordID) {
				return nil, nil, errors.New("Use exact owner, requirement version and record IDs")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.governance(ctx, token, "authrail_list_evidence_record_decisions", controlOwnerPath(input.ControlID, input.ControlVersionID)+"/evidence-requirements/"+input.RequirementID+"/records/"+input.RecordID+"/decisions?"+query, identity)
			return nil, data, err
		})
}
func controlOwnerPath(controlID, versionID string) string {
	return "/organisation/mcp/controls/" + controlID + "/versions/" + versionID
}
func historyQuery(limit int, cursor, implementation string) (string, error) {
	if limit < 0 || limit > 10 || len(cursor) > 500 || implementation != "" && !frameworkID.MatchString(implementation) {
		return "", errors.New("Use page size 1–10 and the previous cursor for this exact owner and tool")
	}
	if limit == 0 {
		limit = 10
	}
	query := url.Values{"limit": {strconv.Itoa(limit)}}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	if implementation != "" {
		query.Set("implementationVersionId", implementation)
	}
	return query.Encode(), nil
}
