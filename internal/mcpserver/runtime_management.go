package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const RuntimeDeploymentsScope = "runtime.deployments"
const RuntimeApprovalsScope = "runtime.approvals"

type deploymentInput struct {
	DeploymentID string `json:"deploymentId"`
}
type deploymentActivationInput struct {
	DeploymentID     string   `json:"deploymentId"`
	RequestKey       string   `json:"requestKey" jsonschema:"UUID reused only for this exact proposal"`
	ExpectedSequence int      `json:"expectedSequence"`
	RailVersionIDs   []string `json:"railVersionIds" jsonschema:"Exact published Rail version UUIDs reviewed by the user"`
	Mode             string   `json:"mode" jsonschema:"observe or enforce; Enforce needs a real human boundary confirmation"`
}
type deploymentRevokeInput struct {
	DeploymentID     string `json:"deploymentId"`
	RequestKey       string `json:"requestKey"`
	ExpectedSequence int    `json:"expectedSequence"`
	Reason           string `json:"reason"`
}
type deploymentRollbackInput struct {
	DeploymentID     string `json:"deploymentId"`
	RequestKey       string `json:"requestKey"`
	ExpectedSequence int    `json:"expectedSequence"`
	Reason           string `json:"reason"`
	SourceBundleID   string `json:"sourceBundleId"`
}

func addRuntimeDeploymentTools(server *mcp.Server, backend *Backend) {
	read := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_deployments", Description: "List visible deployments for one exact AI Inventory item. Requires separate deployment consent and current AI Inventory read permission. Configured identity is not proof of execution identity.", Annotations: read}, func(ctx context.Context, _ *mcp.CallToolRequest, input struct {
		ItemID string `json:"itemId"`
		Cursor string `json:"cursor,omitempty"`
		Limit  int    `json:"limit,omitempty"`
	}) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.ItemID) || input.Cursor != "" && !frameworkID.MatchString(input.Cursor) || input.Limit < 0 || input.Limit > 100 {
			return nil, nil, errors.New("Use an exact AI system ID, optional UUID cursor and limit at most 100")
		}
		query := url.Values{}
		if input.Cursor != "" {
			query.Set("cursor", input.Cursor)
		}
		if input.Limit > 0 {
			query.Set("limit", strconv.Itoa(input.Limit))
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedRead(ctx, token, "authrail_list_deployments", "/organisation/mcp/runtime/ai-systems/"+input.ItemID+"/deployments?"+query.Encode(), RuntimeDeploymentsScope, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_create_deployment", Description: "Create a named Observe installation for an existing authorised AI system. This prepares installation metadata only, issues no credentials and activates no Rails. Use the same UUID only for identical retries; actual AI Inventory manage permission is checked."}, func(ctx context.Context, _ *mcp.CallToolRequest, input struct {
		ItemID         string `json:"itemId"`
		Name           string `json:"name"`
		IdempotencyKey string `json:"idempotencyKey"`
	}) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.ItemID) || !frameworkID.MatchString(input.IdempotencyKey) || strings.TrimSpace(input.Name) == "" || len(input.Name) > 120 {
			return nil, nil, errors.New("Supply exact AI system ID, deployment name and UUID creation key")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedCall(ctx, token, "authrail_create_deployment", http.MethodPost, "/organisation/mcp/runtime/ai-systems/"+input.ItemID+"/deployments", RuntimeDeploymentsScope, map[string]string{"name": input.Name, "idempotencyKey": input.IdempotencyKey}, identity)
		return nil, data, err
	})
	for _, spec := range []struct{ name, path, description string }{
		{"authrail_get_deployment", "", "Read visible deployment metadata and connection signals, without credentials. A healthy connection does not establish protected coverage."},
		{"authrail_get_deployment_setup", "/setup", "Read exact selected integration metadata, public fingerprints and executor key status. Private keys and plaintext deployment credentials are never returned. Run the supported local installer with actual local authority."},
		{"authrail_get_deployment_rail_selection", "/activation", "Read exact currently selected published Rails, mode and sequence. Requires AI Inventory manage and Rail publish permissions; Policy Rails also require Policy read. Readiness still requires human boundary confirmation."},
	} {
		mcp.AddTool(server, &mcp.Tool{Name: spec.name, Description: spec.description, Annotations: read}, func(ctx context.Context, _ *mcp.CallToolRequest, input deploymentInput) (*mcp.CallToolResult, map[string]any, error) {
			if !frameworkID.MatchString(input.DeploymentID) {
				return nil, nil, errors.New("Supply the exact deployment ID")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.delegatedRead(ctx, token, spec.name, "/organisation/mcp/runtime/deployments/"+input.DeploymentID+spec.path, RuntimeDeploymentsScope, identity)
			return nil, data, err
		})
	}
	for _, spec := range []struct{ name, kind string }{{"authrail_request_deployment_activation", "activate"}, {"authrail_request_deployment_renewal", "renew"}} {
		mcp.AddTool(server, &mcp.Tool{Name: spec.name, Description: "Prepare an exact deployment " + spec.kind + " for the signed-in human's review. Returns an expiring account/workspace/version/sequence-bound link, never applies the change. The human must check the proposed Rails and personally confirm protected tool/credential isolation for Enforce. A changed sequence needs a new proposal; never assert human confirmation."}, func(ctx context.Context, _ *mcp.CallToolRequest, input deploymentActivationInput) (*mcp.CallToolResult, map[string]any, error) {
			if !validActivation(input) || spec.kind == "renew" && (input.ExpectedSequence < 1 || input.Mode != "enforce") {
				return nil, nil, errors.New("Supply exact deployment, request key, sequence, published Rail versions and mode")
			}
			body := map[string]any{"requestKey": input.RequestKey, "expectedSequence": input.ExpectedSequence, "railVersionIds": input.RailVersionIDs}
			if spec.kind == "activate" {
				body["mode"] = input.Mode
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.delegatedCall(ctx, token, spec.name, http.MethodPost, "/organisation/mcp/runtime/deployments/"+input.DeploymentID+"/reviews/"+spec.kind, RuntimeDeploymentsScope, body, identity)
			return nil, data, err
		})
	}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_request_deployment_revocation", Description: "Prepare removal of the active Rails from an Observe deployment. Returns an exact authenticated human review link; no Rails are removed by this tool. Revocation cannot turn off Enforce protection through this route."}, func(ctx context.Context, _ *mcp.CallToolRequest, input deploymentRevokeInput) (*mcp.CallToolResult, map[string]any, error) {
		if !validRevocation(input) {
			return nil, nil, errors.New("Supply exact deployment ID, request UUID, current sequence and reason")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedCall(ctx, token, "authrail_request_deployment_revocation", http.MethodPost, "/organisation/mcp/runtime/deployments/"+input.DeploymentID+"/reviews/revoke", RuntimeDeploymentsScope, map[string]any{"requestKey": input.RequestKey, "expectedSequence": input.ExpectedSequence, "reason": input.Reason}, identity)
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_request_deployment_rollback", Description: "Prepare restoration of an exact retained Observe Rail bundle with its original version selection. The authenticated human reviews the target and impact before a newly signed sequence is created. Does not mutate protection or restore an Enforce bundle."}, func(ctx context.Context, _ *mcp.CallToolRequest, input deploymentRollbackInput) (*mcp.CallToolResult, map[string]any, error) {
		if !validRevocation(deploymentRevokeInput{input.DeploymentID, input.RequestKey, input.ExpectedSequence, input.Reason}) || !frameworkID.MatchString(input.SourceBundleID) {
			return nil, nil, errors.New("Supply exact deployment, request UUID, current sequence, retained source bundle and reason")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedCall(ctx, token, "authrail_request_deployment_rollback", http.MethodPost, "/organisation/mcp/runtime/deployments/"+input.DeploymentID+"/reviews/rollback", RuntimeDeploymentsScope, map[string]any{"requestKey": input.RequestKey, "expectedSequence": input.ExpectedSequence, "reason": input.Reason, "sourceBundleId": input.SourceBundleID}, identity)
		return nil, data, err
	})
}

func validActivation(input deploymentActivationInput) bool {
	if !frameworkID.MatchString(input.DeploymentID) || !frameworkID.MatchString(input.RequestKey) || input.ExpectedSequence < 0 || len(input.RailVersionIDs) < 1 || len(input.RailVersionIDs) > 100 || (input.Mode != "observe" && input.Mode != "enforce") {
		return false
	}
	seen := map[string]bool{}
	for _, id := range input.RailVersionIDs {
		if !frameworkID.MatchString(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func validRevocation(input deploymentRevokeInput) bool {
	return frameworkID.MatchString(input.DeploymentID) && frameworkID.MatchString(input.RequestKey) && input.ExpectedSequence > 0 && strings.TrimSpace(input.Reason) != "" && len(input.Reason) <= 500
}

func addRuntimeApprovalTools(server *mcp.Server, backend *Backend) {
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_my_runtime_approvals", Description: "List up to 30 pending approvals assigned to your exact active reviewer membership. Separate approval consent is required; AI Inventory read/publication permission is not. Returns minimal metadata and immutable review hash, never application review context. Approval is permission, not execution or automatic resume.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, _ *mcp.CallToolRequest, input struct {
		Cursor string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, map[string]any, error) { if input.Cursor != "" && !frameworkID.MatchString(input.Cursor) {
		return nil, nil, errors.New("Use the returned approval cursor")
	}; q := url.Values{}; if input.Cursor != "" {
		q.Set("cursor", input.Cursor)
	}; token, identity := requestIdentity(ctx); data, err := backend.delegatedRead(ctx, token, "authrail_list_my_runtime_approvals", "/organisation/mcp/runtime/approval-requests?"+q.Encode(), RuntimeApprovalsScope, identity); return nil, data, err })
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_request_runtime_approval_decision", Description: "Prepare one exact approve, reject or cancel decision for your authenticated human review. Supply the immutable review hash returned by your assigned approval list. Returns an account-bound expiring link where the human reads the application context and personally confirms; this assistant tool cannot vote, obtain an execution permit or resume the customer's work."}, func(ctx context.Context, _ *mcp.CallToolRequest, input struct {
		RequestID           string `json:"requestId"`
		ReviewSummarySHA256 string `json:"reviewSummarySha256"`
		Decision            string `json:"decision"`
	}) (*mcp.CallToolResult, map[string]any, error) { if !frameworkID.MatchString(input.RequestID) || len(input.ReviewSummarySHA256) != 64 || strings.Trim(input.ReviewSummarySHA256, "0123456789abcdef") != "" || (input.Decision != "approve" && input.Decision != "reject" && input.Decision != "cancel") {
		return nil, nil, errors.New("Supply exact assigned request, review hash and intended decision")
	}; token, identity := requestIdentity(ctx); data, err := backend.delegatedCall(ctx, token, "authrail_request_runtime_approval_decision", http.MethodPost, "/organisation/mcp/runtime/approval-requests/"+input.RequestID+"/review", RuntimeApprovalsScope, map[string]string{"reviewSummarySha256": input.ReviewSummarySHA256, "decision": input.Decision}, identity); return nil, data, err })
}
