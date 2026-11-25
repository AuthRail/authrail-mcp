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

func TestBusinessAccountsKeepReadConsentSeparateFromHumanHandoff(t *testing.T) {
	const connectionID = "00000000-0000-4000-8000-000000000001"
	for _, scope := range []string{ConnectionsScope, ConnectionsManageScope} {
		t.Run(scope, func(t *testing.T) {
			var delegated atomic.Int32
			var handoffs atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/auth/mcp/introspect":
					_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user", SessionID: "session", OrganisationID: "org", ClientID: "client", Scopes: []string{Scope, scope}, ExpiresAt: time.Now().Add(time.Minute)})
				case "/auth/mcp/delegations/exchange":
					var input struct{ Token, Tool string }
					if json.NewDecoder(r.Body).Decode(&input) != nil || !strings.HasPrefix(input.Tool, "authrail_") || input.Token != strings.Repeat("u", 32) || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) {
						t.Error("missing named tool delegation")
						w.WriteHeader(403)
						return
					}
					delegated.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "business-handoff", "expiresAt": time.Now().Add(time.Minute)})
				case "/organisation/mcp/connections/business/providers", "/organisation/mcp/connections/business/accounts", "/organisation/mcp/connections/business/accounts/" + connectionID, "/organisation/mcp/connections/business/handoff":
					if r.Header.Get("Authorization") != "Bearer business-handoff" || r.Header.Get("X-Organisation-Id") != "org" {
						t.Error("missing workspace delegation")
						w.WriteHeader(403)
						return
					}
					data := map[string]any{"records": []any{}, "pageInfo": map[string]any{"hasNextPage": false}}
					if strings.HasSuffix(r.URL.Path, "/handoff") {
						var body map[string]any
						if json.NewDecoder(r.Body).Decode(&body) != nil || (len(body) != 2 && len(body) != 3) || r.Method != http.MethodPost ||
							!(body["action"] == "review" && body["connectionId"] == connectionID || body["action"] == "connect" && (body["providerKey"] == "hubspot" || body["providerKey"] == "github")) {
							t.Error("unsafe handoff payload")
							w.WriteHeader(400)
							return
						}
						if body["setup"] != nil {
							setup, ok := body["setup"].(map[string]any)
							if !ok || setup["displayName"] != "GitHub test" || setup["boundary"].(map[string]any)["organisationLogin"] != "AuthRail" {
								t.Error("prepared account details were changed")
							}
						}
						handoffs.Add(1)
						data = map[string]any{"handoff": map[string]any{"url": "https://app.example.test/connections/business/accounts/" + connectionID, "requiresHuman": true}}
					} else if r.Method != http.MethodGet {
						t.Error("saved read mutated state")
					}
					if (r.URL.Path == "/organisation/mcp/connections/business/accounts" || r.URL.Path == "/organisation/mcp/connections/business/providers") && (r.URL.Query().Get("limit") != "3" || r.URL.Query().Get("cursor") != "page+2") {
						t.Error("incorrect bounded query")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": "org", "userId": "user", "data": data})
				default:
					t.Errorf("unexpected connection route %s", r.URL.Path)
					w.WriteHeader(404)
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
			client := mcp.NewClient(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
			session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: resource, HTTPClient: &http.Client{Transport: bearerTransport{strings.Repeat("u", 32)}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			inventory, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			for _, tool := range inventory.Tools {
				found[tool.Name] = true
			}
			if found["authrail_list_business_providers"] != (scope == ConnectionsScope) || found["authrail_list_business_accounts"] != (scope == ConnectionsScope) || found["authrail_get_business_account"] != (scope == ConnectionsScope) || found["authrail_manage_business_account"] != (scope == ConnectionsManageScope) {
				t.Fatal("incorrect business account consent boundary")
			}
			call := func(name string, args map[string]any, wantError bool) {
				t.Helper()
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
				failed := err != nil || result.IsError
				if failed != wantError {
					t.Fatalf("%s failed=%v: %v", name, failed, err)
				}
			}
			if scope == ConnectionsScope {
				call("authrail_list_business_providers", map[string]any{"limit": 3, "cursor": "page+2"}, false)
				call("authrail_list_business_accounts", map[string]any{"limit": 3, "cursor": "page+2"}, false)
				call("authrail_get_business_account", map[string]any{"connectionId": connectionID}, false)
				call("authrail_list_business_accounts", map[string]any{"limit": 11}, true)
				call("authrail_get_business_account", map[string]any{"connectionId": "../../credentials"}, true)
				call("authrail_manage_business_account", map[string]any{"action": "review", "connectionId": connectionID}, true)
				if delegated.Load() != 3 || handoffs.Load() != 0 {
					t.Fatal("invalid/read-only request reached management")
				}
			} else {
				call("authrail_manage_business_account", map[string]any{"action": "review", "connectionId": connectionID}, false)
				call("authrail_manage_business_account", map[string]any{"action": "connect", "providerKey": "hubspot"}, false)
				call("authrail_manage_business_account", map[string]any{"action": "connect", "providerKey": "github", "setup": map[string]any{"displayName": "GitHub test", "boundary": map[string]any{"organisationLogin": "AuthRail"}, "access": []string{"metadata:read", "pull_requests:write"}}}, false)
				for _, args := range []map[string]any{{"action": "connect", "connectionId": connectionID}, {"action": "review"}, {"action": "connect", "providerKey": "unknown"}, {"action": "access", "connectionId": connectionID, "providerKey": "stripe"}, {"action": "review", "connectionId": connectionID, "endpoint": "https://unreviewed.example.test"}, {"action": "disconnect", "connectionId": connectionID, "credential": "secret"},
					{"action": "connect", "providerKey": "github", "setup": map[string]any{"credential": "secret"}},
					{"action": "connect", "providerKey": "github", "setup": map[string]any{"boundary": map[string]any{"clientSecret": "secret"}}},
					{"action": "connect", "providerKey": "github", "setup": map[string]any{"boundary": map[string]any{"origin": "https://unreviewed.example"}}},
					{"action": "review", "connectionId": connectionID, "setup": map[string]any{"displayName": "Wrong action"}}} {
					call("authrail_manage_business_account", args, true)
				}
				if delegated.Load() != 3 || handoffs.Load() != 3 {
					t.Fatal("invalid handoff reached account API")
				}
			}
		})
	}
}
