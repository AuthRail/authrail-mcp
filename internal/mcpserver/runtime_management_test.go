package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Reserved decisions must remain proposals, with separate consent and no model-supplied human assertion.
func TestRuntimeManagementConsentAndHumanHandoff(t *testing.T) {
	for _, scope := range []string{"", RuntimeDeploymentsScope, RuntimeApprovalsScope} {
		t.Run(scope, func(t *testing.T) {
			var preparations atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/auth/mcp/introspect":
					scopes := []string{Scope, RuntimeReadScope, RuntimeDraftsScope}
					if scope != "" {
						scopes = append(scopes, scope)
					}
					_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user-1", SessionID: "session-1", OrganisationID: "org-1", ClientID: "client-1", Scopes: scopes, ExpiresAt: time.Now().Add(time.Minute)})
				case "/auth/mcp/delegations/exchange":
					var input struct{ Token, Tool string }
					_ = json.NewDecoder(r.Body).Decode(&input)
					tool := "authrail_request_deployment_activation"
					if scope == RuntimeApprovalsScope {
						tool = "authrail_request_runtime_approval_decision"
					}
					if input.Tool != tool || input.Token != strings.Repeat("u", 32) || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) {
						t.Error("wrong named delegated authority")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "named-delegation", "expiresAt": time.Now().Add(time.Minute)})
				case "/organisation/mcp/runtime/deployments/00000000-0000-4000-8000-000000000001/reviews/activate", "/organisation/mcp/runtime/approval-requests/00000000-0000-4000-8000-000000000001/review":
					if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer named-delegation" || r.Header.Get("X-Organisation-Id") != "org-1" {
						t.Error("wrong user/tenant handoff binding")
					}
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					if _, found := body["confirmedProtectedBoundary"]; found {
						t.Error("agent submitted a human boundary assertion")
					}
					preparations.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": "org-1", "userId": "user-1", "data": map[string]any{"confirmationRequired": true, "reviewUrl": "https://workspace.example.invalid/runtime/review#review=signed"}})
				default:
					t.Errorf("unexpected executable or unrelated route %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer api.Close()
			endpoint := httptest.NewUnstartedServer(nil)
			resource := "http://" + endpoint.Listener.Addr().String() + "/mcp"
			backend, err := NewBackend(Config{APIURL: api.URL, ResourceURL: resource, ServiceToken: strings.Repeat("s", 32), AllowLoopback: true})
			if err != nil {
				t.Fatal(err)
			}
			endpoint.Config.Handler = Handler(backend)
			endpoint.Start()
			defer endpoint.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			client := mcp.NewClient(&mcp.Implementation{Name: "verification", Version: "1"}, nil)
			session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: resource, HTTPClient: &http.Client{Transport: bearerTransport{strings.Repeat("u", 32)}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			inventory, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			foundDeployment, foundApproval := false, false
			for _, tool := range inventory.Tools {
				if tool.Name == "authrail_request_deployment_activation" {
					foundDeployment = true
				}
				if tool.Name == "authrail_request_runtime_approval_decision" {
					foundApproval = true
				}
			}
			if foundDeployment != (scope == RuntimeDeploymentsScope) || foundApproval != (scope == RuntimeApprovalsScope) {
				t.Fatal("Runtime consent groups leaked into unrelated scopes")
			}
			if scope == "" {
				return
			}
			tool := "authrail_request_deployment_activation"
			arguments := map[string]any{"deploymentId": "00000000-0000-4000-8000-000000000001", "requestKey": "00000000-0000-4000-8000-000000000002", "expectedSequence": 0, "railVersionIds": []string{"00000000-0000-4000-8000-000000000003"}, "mode": "enforce"}
			if scope == RuntimeApprovalsScope {
				tool = "authrail_request_runtime_approval_decision"
				arguments = map[string]any{"requestId": "00000000-0000-4000-8000-000000000001", "reviewSummarySha256": strings.Repeat("a", 64), "decision": "approve"}
			}
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: arguments})
			if err != nil || result.IsError {
				t.Fatalf("handoff: %v %v", result, err)
			}
			if preparations.Load() != 1 {
				t.Fatal("proposal did not reach exact delegated preparation")
			}
			arguments["confirmedProtectedBoundary"] = true
			result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: arguments})
			if err == nil && !result.IsError {
				t.Fatal("agent supplied reserved confirmation")
			}
			if preparations.Load() != 1 {
				t.Fatal("invalid assertion reached backend")
			}
		})
	}
}
