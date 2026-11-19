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

func TestBusinessExecutionUsesSeparateConsentSavedIdentityAndProtectedResults(t *testing.T) {
	const routeID = "00000000-0000-4000-8000-000000000001"
	const requestID = "00000000-0000-4000-8000-000000000002"
	for _, consent := range []bool{false, true} {
		t.Run(map[bool]string{false: "discovery-only", true: "execution-consented"}[consent], func(t *testing.T) {
			var delegated, businessCalls atomic.Int32
			var wrongIdentity, wrongRequest, leakResult atomic.Bool
			var activeConsent atomic.Bool
			activeConsent.Store(consent)
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/auth/mcp/introspect":
					scopes := []string{Scope, BusinessToolsReadScope}
					if activeConsent.Load() {
						scopes = append(scopes, BusinessExecuteScope)
					}
					_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user", SessionID: "session", OrganisationID: "org", ClientID: "client", Scopes: scopes, ExpiresAt: time.Now().Add(time.Minute)})
				case "/auth/mcp/delegations/exchange":
					var input struct{ Token, Tool string }
					if json.NewDecoder(r.Body).Decode(&input) != nil || input.Token != strings.Repeat("u", 32) || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) {
						t.Error("incorrect named service delegation")
						w.WriteHeader(http.StatusForbidden)
						return
					}
					delegated.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "delegation-" + input.Tool, "expiresAt": time.Now().Add(time.Minute)})
				case "/organisation/mcp/business-requests", "/organisation/mcp/business-requests/" + requestID, "/organisation/mcp/business-requests/" + requestID + "/resume", "/organisation/mcp/business-requests/" + requestID + "/cancel":
					businessCalls.Add(1)
					name := "authrail_get_business_request"
					if r.URL.Path == "/organisation/mcp/business-requests" {
						name = "authrail_request_business_action"
						var input businessRequestInput
						if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&input) != nil || input.RouteID != routeID || input.BindingSHA256 != strings.Repeat("a", 64) || input.IdempotencyKey != requestID || input.Arguments["title"] != "Reviewed title" {
							t.Error("changed exact saved-work proposal")
						}
					} else if strings.HasSuffix(r.URL.Path, "/resume") || strings.HasSuffix(r.URL.Path, "/cancel") {
						name = "authrail_resume_business_request"
						if strings.HasSuffix(r.URL.Path, "/cancel") {
							name = "authrail_cancel_business_request"
						}
						var input map[string]int
						if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&input) != nil || len(input) != 1 || input["expectedRevision"] != 3 {
							t.Error("resume/cancel changed the saved request")
						}
					} else if r.Method != http.MethodGet {
						t.Error("status read used a mutating method")
					}
					if r.Header.Get("Authorization") != "Bearer delegation-"+name || r.Header.Get("X-Organisation-Id") != "org" {
						t.Error("lost named delegation or authenticated tenant")
					}
					org, id := "org", requestID
					if wrongIdentity.Load() {
						org = "other-org"
					}
					if wrongRequest.Load() {
						id = routeID
					}
					data := map[string]any{"requestId": id, "revision": 3, "status": "awaiting_approval", "resultReleased": false}
					if leakResult.Load() {
						data["result"] = "PRIVATE-UNRELEASED-RESULT"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": org, "userId": "user", "data": data})
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
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
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			client := mcp.NewClient(&mcp.Implementation{Name: "business-execution-fixture", Version: "1"}, nil)
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
			for _, name := range []string{"authrail_request_business_action", "authrail_get_business_request", "authrail_resume_business_request", "authrail_cancel_business_request"} {
				if found[name] != consent {
					t.Fatalf("wrong execution consent for %s", name)
				}
			}
			call := func(name string, args map[string]any, failure bool) {
				t.Helper()
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
				if failed := err != nil || result.IsError; failed != failure {
					t.Fatalf("%s failed=%v: %v %v", name, failed, result, err)
				}
				encoded, _ := json.Marshal(result)
				if strings.Contains(string(encoded), "PRIVATE-UNRELEASED") {
					t.Fatal("released unchecked provider result")
				}
			}
			proposal := map[string]any{"routeId": routeID, "bindingSha256": strings.Repeat("a", 64), "idempotencyKey": requestID, "arguments": map[string]any{"title": "Reviewed title"}}
			call("authrail_request_business_action", proposal, !consent)
			if !consent {
				if delegated.Load() != 0 || businessCalls.Load() != 0 {
					t.Fatal("discovery consent executed work")
				}
				return
			}
			call("authrail_get_business_request", map[string]any{"requestId": requestID}, false)
			for _, name := range []string{"authrail_resume_business_request", "authrail_cancel_business_request"} {
				call(name, map[string]any{"requestId": requestID, "expectedRevision": 3}, false)
				call(name, map[string]any{"requestId": requestID}, true)
				call(name, map[string]any{"requestId": requestID, "expectedRevision": 3, "arguments": map[string]any{"title": "Replacement"}}, true)
			}
			for _, field := range []string{"accountId", "workloadId", "principal", "endpoint", "credential", "organisationId"} {
				invalid := map[string]any{}
				for key, value := range proposal {
					invalid[key] = value
				}
				invalid[field] = "agent-supplied-authority"
				call("authrail_request_business_action", invalid, true)
			}
			if delegated.Load() != 4 || businessCalls.Load() != 4 {
				t.Fatal("pending response polled/resumed automatically or invalid input reached API")
			}
			wrongIdentity.Store(true)
			call("authrail_get_business_request", map[string]any{"requestId": requestID}, true)
			wrongIdentity.Store(false)
			wrongRequest.Store(true)
			call("authrail_get_business_request", map[string]any{"requestId": requestID}, true)
			wrongRequest.Store(false)
			leakResult.Store(true)
			call("authrail_get_business_request", map[string]any{"requestId": requestID}, true)
			activeConsent.Store(false)
			call("authrail_get_business_request", map[string]any{"requestId": requestID}, true)
			if businessCalls.Load() != 7 {
				t.Fatal("withdrawn execution consent retained execution authority")
			}
		})
	}
}

func TestBusinessResultReleaseRequiresExplicitSuccess(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{"requestId": "00000000-0000-4000-8000-000000000001", "revision": float64(1), "status": "succeeded", "resultReleased": true, "result": map[string]any{"text": "Approved result"}}
	}
	if _, err := safeBusinessRequest(base()); err != nil {
		t.Fatal(err)
	}
	for _, change := range []map[string]any{{"status": "outcome_unknown"}, {"resultReleased": false}, {"revision": 1.5}, {"providerCredential": "private"}, {"requiresRefresh": "false"}, {"actionId": "invalid"}} {
		data := base()
		for key, value := range change {
			data[key] = value
		}
		if _, err := safeBusinessRequest(data); err == nil {
			t.Fatalf("accepted unsafe response fields: %v", change)
		}
	}
}
