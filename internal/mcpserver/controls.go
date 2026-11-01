package mcpserver

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mappingListInput struct {
	OrganisationFrameworkID           string `json:"organisationFrameworkId" jsonschema:"Selected Framework workspace ID"`
	ObligationApplicabilityDecisionID string `json:"obligationApplicabilityDecisionId" jsonschema:"Exact Obligation decision ID returned by the readiness tool"`
	Limit                             int    `json:"limit,omitempty" jsonschema:"Page size from 1 to 10; defaults to 10"`
	Cursor                            string `json:"cursor,omitempty" jsonschema:"Opaque cursor from the previous mapping page"`
}
type controlVersionInput struct {
	ControlID        string `json:"controlId" jsonschema:"Control ID returned by a visible mapping"`
	ControlVersionID string `json:"controlVersionId" jsonschema:"Exact Control version ID; do not substitute a newer draft"`
}
type verificationListInput struct {
	ControlID               string `json:"controlId" jsonschema:"Control ID returned by a visible mapping"`
	ControlVersionID        string `json:"controlVersionId" jsonschema:"Exact Control version ID"`
	ImplementationVersionID string `json:"implementationVersionId,omitempty" jsonschema:"Exact scoped Implementation version when inspecting that owner"`
	Limit                   int    `json:"limit,omitempty" jsonschema:"Page size from 1 to 10; defaults to 10"`
	Cursor                  string `json:"cursor,omitempty" jsonschema:"Opaque cursor from the previous page for this exact owner and tool"`
}

func addControlTools(server *mcp.Server, backend *Backend) {
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_control_mappings", Description: "Follow an Obligation decision into visible Control mappings and their lifecycle. Restricted means some mappings could not be disclosed; do not treat the visible list as complete coverage. This includes history, which must not be counted as active coverage.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input mappingListInput) (*mcp.CallToolResult, map[string]any, error) {
			if !frameworkID.MatchString(input.OrganisationFrameworkID) || !frameworkID.MatchString(input.ObligationApplicabilityDecisionID) || input.Limit < 0 || input.Limit > 10 || len(input.Cursor) > 500 {
				return nil, nil, errors.New("Use the exact Framework and Obligation decision IDs, page size 1–10 and previous mapping cursor")
			}
			limit := input.Limit
			if limit == 0 {
				limit = 10
			}
			q := url.Values{"organisationFrameworkId": {input.OrganisationFrameworkID}, "obligationApplicabilityDecisionId": {input.ObligationApplicabilityDecisionID}, "limit": {strconv.Itoa(limit)}}
			if input.Cursor != "" {
				q.Set("cursor", input.Cursor)
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.governance(ctx, token, "authrail_list_control_mappings", "/organisation/mcp/control-mappings?"+q.Encode(), identity)
			return nil, data, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_control", Description: "Read metadata for the exact Control version named by a mapping. This does not select a newer draft or disclose private statement/source bodies.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input controlVersionInput) (*mcp.CallToolResult, map[string]any, error) {
			if !frameworkID.MatchString(input.ControlID) || !frameworkID.MatchString(input.ControlVersionID) {
				return nil, nil, errors.New("Use both the Control ID and exact Control version ID")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.governance(ctx, token, "authrail_get_control", "/organisation/mcp/controls/"+input.ControlID+"/versions/"+input.ControlVersionID, identity)
			return nil, data, err
		})
	for _, binding := range []struct{ name, path, description string }{
		{"authrail_list_tests", "tests", "Read Test definitions, latest reported run outcomes and freshness for an exact Control owner. A missing, failed, errored, stale or restricted Test is not passing proof. Private procedures and run payloads are excluded."},
		{"authrail_list_evidence_requirements", "evidence-requirements", "Read the proof requirements for an exact Control owner. This returns requirement metadata, not accepted Evidence, file contents or download links."},
	} {
		mcp.AddTool(server, &mcp.Tool{Name: binding.name, Description: binding.description, Annotations: annotations},
			func(ctx context.Context, _ *mcp.CallToolRequest, input verificationListInput) (*mcp.CallToolResult, map[string]any, error) {
				if !frameworkID.MatchString(input.ControlID) || !frameworkID.MatchString(input.ControlVersionID) || input.ImplementationVersionID != "" && !frameworkID.MatchString(input.ImplementationVersionID) || input.Limit < 0 || input.Limit > 10 || len(input.Cursor) > 500 {
					return nil, nil, errors.New("Use exact owner IDs, page size 1–10 and the previous cursor for this tool")
				}
				limit := input.Limit
				if limit == 0 {
					limit = 10
				}
				q := url.Values{"limit": {strconv.Itoa(limit)}}
				if input.Cursor != "" {
					q.Set("cursor", input.Cursor)
				}
				if input.ImplementationVersionID != "" {
					q.Set("implementationVersionId", input.ImplementationVersionID)
				}
				token, identity := requestIdentity(ctx)
				data, err := backend.governance(ctx, token, binding.name, "/organisation/mcp/controls/"+input.ControlID+"/versions/"+input.ControlVersionID+"/"+binding.path+"?"+q.Encode(), identity)
				return nil, data, err
			})
	}
}
