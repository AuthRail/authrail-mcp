package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPolicyDiscoveryKeepsContentAndDraftConsentSeparate(t *testing.T) {
	for _, scope := range []string{PoliciesReadScope, PoliciesManageScope} {
		t.Run(scope, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path != "/auth/mcp/introspect" {
					t.Errorf("unexpected dispatch %s", r.URL.Path)
					w.WriteHeader(403)
					return
				}
				_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user", SessionID: "session", OrganisationID: "org", ClientID: "client", Scopes: []string{Scope, scope}, ExpiresAt: time.Now().Add(time.Minute)})
			}))
			defer api.Close()
			endpoint := httptest.NewUnstartedServer(nil)
			resource := "http://" + endpoint.Listener.Addr().String()
			backend, err := NewBackend(Config{APIURL: api.URL, ResourceURL: resource, ServiceToken: strings.Repeat("s", 32), AllowLoopback: true})
			if err != nil {
				t.Fatal(err)
			}
			endpoint.Config.Handler = Handler(backend)
			endpoint.Start()
			defer endpoint.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			client := mcp.NewClient(&mcp.Implementation{Name: "policy-fixture", Version: "1"}, nil)
			session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: resource, HTTPClient: &http.Client{Transport: bearerTransport{strings.Repeat("u", 32)}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			tools, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			for _, tool := range tools.Tools {
				found[tool.Name] = true
			}
			if found["authrail_get_policy_content"] != (scope == PoliciesReadScope) || found["authrail_save_policy_draft"] != (scope == PoliciesManageScope) || found["authrail_prepare_policy_handoff"] != (scope == PoliciesManageScope) {
				t.Fatal("Policy read/manage discovery crossed consent")
			}
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_prepare_policy_handoff", Arguments: map[string]any{"action": "publish", "policyId": "../../secret", "revision": 1}})
			if err == nil && !result.IsError {
				t.Fatal("invalid or unconsented handoff was accepted")
			}
		})
	}
}
func TestPolicyAnswerKeepsFalseAndRejectsCompetingFormats(t *testing.T) {
	value := false
	text := "competing"
	input := policyAuthoringTurnInput{SessionID: "00000000-0000-4000-8000-000000000001", IdempotencyKey: "00000000-0000-4000-8000-000000000002", Revision: 2, Answers: []policyAuthoringAnswer{{QuestionID: "human-review", Boolean: &value}}}
	_, body, err := preparePolicyAnswer(input)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(body)
	if !strings.Contains(string(encoded), `"value":false`) {
		t.Fatal("false guided answer was lost")
	}
	input.Answers[0].Text = &text
	if _, _, err := preparePolicyAnswer(input); err == nil {
		t.Fatal("competing guided formats accepted")
	}
}
