package mcpserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProposalFailureOnlyExposesKnownDomainCodes(t *testing.T) {
	for _, tc := range []struct{ code, want string }{
		{"rail_proposal_invalid", "did not pass validation"},
		{"rail_proposal_unavailable", "without reconnecting"},
		{"rail_proposal_input_limit", "exceeds the proposal limits"},
		{"unexpected_secret", errBackend.Error()},
	} {
		t.Run(tc.code, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprintf(w, `{"code":%q,"message":"private-provider-diagnostic"}`, tc.code)
			}))
			defer s.Close()
			b := &Backend{origin: s.URL, client: s.Client()}
			var output map[string]any
			err := b.call(context.Background(), http.MethodPost, "/organisation/mcp/policy-rails/proposals", "token", "org", map[string]string{}, &output)
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "private-provider-diagnostic") {
				t.Fatalf("unsafe or missing diagnostic: %v", err)
			}
		})
	}
}

type proposalTransport func(*http.Request) (*http.Response, error)

func (f proposalTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPolicyProposalDeadlineOutlastsProviderBudget(t *testing.T) {
	b := &Backend{origin: "https://api.example", serviceToken: "service", client: &http.Client{Timeout: 10 * time.Second}}
	b.client.Transport = proposalTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"accessToken":"delegated","expiresAt":"2099-01-01T00:00:00Z"}`
		if r.URL.Path != "/auth/mcp/delegations/exchange" {
			deadline, ok := r.Context().Deadline()
			if !ok || time.Until(deadline) < 24*time.Second || time.Until(deadline) > 25*time.Second {
				t.Errorf("proposal deadline must leave room for bounded provider call")
			}
			body = `{"organisationId":"org","userId":"user","data":{"proposal":{}}}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	_, err := b.runtimeDraft(context.Background(), "token", "authrail_propose_policy_rail", http.MethodPost, "/organisation/mcp/policy-rails/proposals", map[string]string{}, Identity{OrganisationID: "org", UserID: "user", Scopes: []string{RuntimeDraftsScope}})
	if err != nil {
		t.Fatal(err)
	}
}
