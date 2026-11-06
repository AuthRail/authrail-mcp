package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RuntimeDraftsScope permits Rail preparation; it never permits publication or execution.
const RuntimeDraftsScope = "runtime.drafts"

type connectedActionListInput struct {
	Limit   int    `json:"limit,omitempty" jsonschema:"Page size 1 to 50; default 30"`
	AfterID string `json:"afterId,omitempty" jsonschema:"Last action ID from the previous page"`
}
type connectedActionInput struct {
	OperationID string `json:"operationId" jsonschema:"Connected action ID returned by authrail_list_connected_actions; this is not its version ID"`
}
type railDraftInput struct {
	RailID    string `json:"railId" jsonschema:"Exact Rail ID"`
	VersionID string `json:"versionId" jsonschema:"Exact version ID returned when the draft was created"`
}

type contextualProposalInput struct {
	Kind               string `json:"kind" jsonschema:"guard or policy; Policy Rails require a published policyVersionId"`
	OperationVersionID string `json:"operationVersionId" jsonschema:"Exact reviewed action contract to protect"`
	PolicyVersionID    string `json:"policyVersionId,omitempty" jsonschema:"Exact published Policy version for a Policy Rail"`
	Instructions       string `json:"instructions" jsonschema:"Describe the business rule and what its selected material should be assessed for, from 10 to 1000 characters. Do not include secrets or model settings"`
}
type createRailInput struct {
	Name               string `json:"name" jsonschema:"Name of the Rail draft"`
	OperationVersionID string `json:"operationVersionId" jsonschema:"Exact reviewed action contract"`
	PolicyVersionID    string `json:"policyVersionId,omitempty" jsonschema:"Required published Policy version when creating a Policy Rail"`
	IdempotencyKey     string `json:"idempotencyKey" jsonschema:"UUID reused only for retries of this identical draft creation"`
}
type saveRailInput struct {
	RailID    string         `json:"railId" jsonschema:"Draft Rail ID returned by AuthRail"`
	VersionID string         `json:"versionId" jsonschema:"Exact draft version ID"`
	Revision  int            `json:"revision" jsonschema:"Expected revision returned by the last read or save"`
	Name      string         `json:"name" jsonschema:"Rail name"`
	Content   map[string]any `json:"content" jsonschema:"Complete reviewed draft content, including rules, examples and the Rail-owned contextual setting returned by preparation; no provider/model fields or credentials"`
}
type testRailInput struct {
	RailID    string `json:"railId" jsonschema:"Draft Rail ID"`
	VersionID string `json:"versionId" jsonschema:"Exact draft version ID"`
	Revision  int    `json:"revision" jsonschema:"Expected saved revision to test"`
}

func addRuntimeDraftTools(server *mcp.Server, backend *Backend) {
	addDataDraftTools(server, backend)
	addActionDraftTools(server, backend)
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_guard_protections", Description: "Choose a built-in Guard for a reviewed request or result field: recognised credentials, common email formats, Luhn-valid payment-card formats, unconditional field withholding, approved destination values, or a contextual assessment of instruction manipulation or sensitive disclosure. Read the returned templates and exact limitations; these are distinct protections, not universal sensitive-data or prompt-injection detection. Local processing runs before permitted contextual disclosure. A contextual preset needs explicit Rail-owned Contextual AI enablement and a reviewed boolean assessment; uncertainty never becomes clearance. Prepare paired deny/allow conditions as the template describes and bind the exact reviewed fields. Save and test the draft, then ask a person to review publication. This tool does not activate a protection, publish a Rail or secure uninstrumented tools.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ noArguments) (*mcp.CallToolResult, map[string]any, error) {
			token, identity := requestIdentity(ctx)
			data, err := backend.runtimeDraft(ctx, token, "authrail_list_guard_protections", http.MethodGet, "/organisation/mcp/rails/guard-protections", nil, identity)
			return nil, data, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_list_connected_actions", Description: "Find connected actions in your workspace before preparing a contextual Rail. Read the selected action to choose its exact reviewed Operation Version and permitted fields. This does not connect a provider or intercept a function.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input connectedActionListInput) (*mcp.CallToolResult, map[string]any, error) {
			if input.Limit < 0 || input.Limit > 50 || input.AfterID != "" && !frameworkID.MatchString(input.AfterID) {
				return nil, nil, errors.New("Use a page size from 1 to 50 and the last action ID")
			}
			limit := input.Limit
			if limit == 0 {
				limit = 30
			}
			query := url.Values{"limit": {strconv.Itoa(limit)}}
			if input.AfterID != "" {
				query.Set("afterId", input.AfterID)
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.runtimeDraft(ctx, token, "authrail_list_connected_actions", http.MethodGet, "/organisation/mcp/rails/actions?"+query.Encode(), nil, identity)
			return nil, data, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_connected_action", Description: "Read one connected action's versions and field contract. Select the reviewed version, not a draft or the parent action ID, when preparing a Rail.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input connectedActionInput) (*mcp.CallToolResult, map[string]any, error) {
			if !frameworkID.MatchString(input.OperationID) {
				return nil, nil, errors.New("Use the connected action ID")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.runtimeDraft(ctx, token, "authrail_get_connected_action", http.MethodGet, "/organisation/mcp/rails/actions/"+input.OperationID, nil, identity)
			return nil, data, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_rail_draft", Description: "Read a Rail's exact version, revision, rules, examples and contextual assessment before editing. After a save conflict, read it again and reconcile changes; never overwrite a concurrent edit. Publication requires a person's review.", Annotations: annotations},
		func(ctx context.Context, _ *mcp.CallToolRequest, input railDraftInput) (*mcp.CallToolResult, map[string]any, error) {
			if !frameworkID.MatchString(input.RailID) || !frameworkID.MatchString(input.VersionID) {
				return nil, nil, errors.New("Use the exact Rail and version IDs")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.runtimeDraft(ctx, token, "authrail_get_rail_draft", http.MethodGet, "/organisation/mcp/rails/"+input.RailID+"/drafts/"+input.VersionID, nil, identity)
			return nil, data, err
		})

	mcp.AddTool(server, &mcp.Tool{Name: "authrail_propose_contextual_rail", Description: "Prepare an unsaved Rail with AuthRail Contextual AI enabled when the rule needs interpretation. AuthRail derives the assessment; users do not configure a model. Review the question and permitted fields. AuthRail interprets the material and the local sidecar applies the published rule; uncertainty is never permission. This tool does not save or publish."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input contextualProposalInput) (*mcp.CallToolResult, map[string]any, error) {
			if input.Kind != "guard" && input.Kind != "policy" || !frameworkID.MatchString(input.OperationVersionID) ||
				len(strings.TrimSpace(input.Instructions)) < 10 || len(input.Instructions) > 1000 ||
				input.Kind == "policy" && !frameworkID.MatchString(input.PolicyVersionID) {
				return nil, nil, errors.New("Select a reviewed action and describe the intended rule; a Policy Rail also needs its exact published Policy version")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.runtimeDraft(ctx, token, "authrail_propose_contextual_rail", http.MethodPost, "/organisation/mcp/rails/contextual-proposals", input, identity)
			return nil, data, err
		})
	for _, kind := range []string{"guard", "policy"} {
		mcp.AddTool(server, &mcp.Tool{Name: "authrail_create_" + kind + "_rail_draft", Description: "Create an unpublished " + kind + " Rail draft using your workspace permissions. Reuse the UUID key for identical creation retries. Save the prepared rules and contextual assessment, test them, then ask a person to review publication in AuthRail. This tool cannot activate or publish a Rail."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input createRailInput) (*mcp.CallToolResult, map[string]any, error) {
				if !frameworkID.MatchString(input.OperationVersionID) || !frameworkID.MatchString(input.IdempotencyKey) || strings.TrimSpace(input.Name) == "" || len(input.Name) > 180 || kind == "policy" && !frameworkID.MatchString(input.PolicyVersionID) {
					return nil, nil, errors.New("Supply a Rail name, exact reviewed action, UUID creation key and any required Policy version")
				}
				body := map[string]any{"name": input.Name, "operationVersionId": input.OperationVersionID, "idempotencyKey": input.IdempotencyKey}
				if kind == "policy" {
					body["policyVersionId"] = input.PolicyVersionID
				}
				token, identity := requestIdentity(ctx)
				data, err := backend.runtimeDraft(ctx, token, "authrail_create_"+kind+"_rail_draft", http.MethodPost, "/organisation/mcp/rails/"+kind+"-drafts", body, identity)
				return nil, data, err
			})
		mcp.AddTool(server, &mcp.Tool{Name: "authrail_save_" + kind + "_rail_draft", Description: "Save an exact " + kind + " Rail draft revision, including its prepared Contextual AI setting and examples. A conflict means fetch and review the new draft before retrying; never overwrite a concurrent human edit. Saving does not publish or protect a tool."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input saveRailInput) (*mcp.CallToolResult, map[string]any, error) {
				if !frameworkID.MatchString(input.RailID) || !frameworkID.MatchString(input.VersionID) || input.Revision < 1 || strings.TrimSpace(input.Name) == "" || len(input.Name) > 180 || input.Content == nil {
					return nil, nil, errors.New("Supply the exact draft IDs, expected revision, name and complete reviewed content")
				}
				body := map[string]any{"revision": input.Revision, "name": input.Name, "content": input.Content}
				token, identity := requestIdentity(ctx)
				data, err := backend.runtimeDraft(ctx, token, "authrail_save_"+kind+"_rail_draft", http.MethodPatch, "/organisation/mcp/rails/"+input.RailID+"/"+kind+"-drafts/"+input.VersionID, body, identity)
				return nil, data, err
			})
		mcp.AddTool(server, &mcp.Tool{Name: "authrail_test_" + kind + "_rail_draft", Description: "Run saved examples for an exact " + kind + " Rail draft revision. Inspect passing, failing and missing-context cases; examples do not independently prove model accuracy or tool interception. After testing, a person must review and publish the exact draft in AuthRail."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input testRailInput) (*mcp.CallToolResult, map[string]any, error) {
				if !frameworkID.MatchString(input.RailID) || !frameworkID.MatchString(input.VersionID) || input.Revision < 1 {
					return nil, nil, errors.New("Supply exact draft IDs and the expected saved revision")
				}
				token, identity := requestIdentity(ctx)
				data, err := backend.runtimeDraft(ctx, token, "authrail_test_"+kind+"_rail_draft", http.MethodPost, "/organisation/mcp/rails/"+input.RailID+"/"+kind+"-drafts/"+input.VersionID+"/test", map[string]int{"revision": input.Revision}, identity)
				return nil, data, err
			})
	}
}

func (b *Backend) runtimeDraft(ctx context.Context, token, tool, method, path string, input any, identity Identity) (map[string]any, error) {
	return b.delegatedCall(ctx, token, tool, method, path, RuntimeDraftsScope, input, identity)
}

func (b *Backend) delegatedCall(ctx context.Context, token, tool, method, path, scope string, input any, identity Identity) (map[string]any, error) {
	if !slices.Contains(identity.Scopes, scope) {
		return nil, errors.New("Reconnect with the required consent and your account's permission for this task")
	}
	var delegation struct {
		AccessToken string    `json:"accessToken"`
		ExpiresAt   time.Time `json:"expiresAt"`
	}
	if err := b.call(ctx, http.MethodPost, "/auth/mcp/delegations/exchange", b.serviceToken, "", map[string]string{"token": token, "tool": tool}, &delegation); err != nil {
		return nil, err
	}
	if delegation.AccessToken == "" || !delegation.ExpiresAt.After(time.Now()) {
		return nil, errBackend
	}
	var result struct {
		OrganisationID string         `json:"organisationId"`
		UserID         string         `json:"userId"`
		Data           map[string]any `json:"data"`
		ReviewURL      string         `json:"reviewUrl"`
	}
	timeout := 10 * time.Second
	if tool == "authrail_propose_contextual_rail" {
		timeout = 25 * time.Second
	}
	if err := b.call(ctx, method, path, delegation.AccessToken, identity.OrganisationID, input, &result, timeout); err != nil {
		return nil, err
	}
	if result.OrganisationID != identity.OrganisationID || result.UserID != identity.UserID || result.Data == nil {
		return nil, errBackend
	}
	if result.ReviewURL != "" {
		result.Data["reviewUrl"] = result.ReviewURL
	}
	return result.Data, nil
}
