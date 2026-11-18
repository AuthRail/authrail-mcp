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

func TestAvailableToolsRequireSeparateConsentAndExactDelegation(t *testing.T) {
	const routeID = "00000000-0000-4000-8000-000000000001"
	const cursorID = "00000000-0000-4000-8000-000000000002"
	for _, consent := range []bool{false, true} {
		name := "without-discovery-consent"
		if consent {
			name = "with-discovery-consent"
		}
		t.Run(name, func(t *testing.T) {
			var delegated atomic.Int32
			var wrongIdentity atomic.Bool
			var discoveryGranted atomic.Bool
			discoveryGranted.Store(consent)
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/auth/mcp/introspect":
					scopes := []string{Scope}
					if discoveryGranted.Load() {
						scopes = append(scopes, BusinessToolsReadScope)
					}
					_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user", SessionID: "session", OrganisationID: "org", ClientID: "client", Scopes: scopes, ExpiresAt: time.Now().Add(time.Minute)})
				case "/auth/mcp/delegations/exchange":
					var input struct{ Token, Tool string }
					if json.NewDecoder(r.Body).Decode(&input) != nil || input.Token != strings.Repeat("u", 32) ||
						r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) ||
						(input.Tool != "authrail_list_available_tools" && input.Tool != "authrail_get_available_tool") {
						t.Error("incorrect tool or service delegation")
						w.WriteHeader(http.StatusForbidden)
						return
					}
					delegated.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "delegation-" + input.Tool, "expiresAt": time.Now().Add(time.Minute)})
				case "/organisation/mcp/business-tools", "/organisation/mcp/business-tools/" + routeID:
					list := r.URL.Path == "/organisation/mcp/business-tools"
					tool := "authrail_get_available_tool"
					if list {
						tool = "authrail_list_available_tools"
						if r.URL.Query().Get("limit") != "3" || r.URL.Query().Get("cursor") != cursorID {
							t.Error("incorrect bounded discovery query")
						}
					}
					if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer delegation-"+tool || r.Header.Get("X-Organisation-Id") != "org" {
						t.Error("discovery lost named delegation or tenant identity")
						w.WriteHeader(http.StatusForbidden)
						return
					}
					summary := map[string]any{"routeId": routeID, "name": "Read issue", "accountName": "Planning", "applicationName": "Support assistant", "provider": "linear", "protocol": "native"}
					data := summary
					if list {
						data = map[string]any{"records": []any{summary}, "pageInfo": map[string]any{"hasNextPage": false, "nextCursor": nil}}
					} else {
						data["bindingSha256"] = strings.Repeat("a", 64)
						data["inputSchema"] = map[string]any{"type": "object", "properties": map[string]any{"issueId": map[string]any{"type": "string"}}}
						data["resultSchema"] = map[string]any{"type": "object"}
					}
					org := "org"
					if wrongIdentity.Load() {
						org = "other-org"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": org, "userId": "user", "data": data})
				default:
					t.Errorf("unexpected discovery route %s", r.URL.Path)
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
			client := mcp.NewClient(&mcp.Implementation{Name: "discovery-fixture", Version: "1"}, nil)
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
			for _, item := range inventory.Tools {
				found[item.Name] = true
			}
			for _, name := range []string{"authrail_list_available_tools", "authrail_get_available_tool"} {
				if found[name] != consent {
					t.Fatalf("incorrect discovery consent for %s", name)
				}
			}
			call := func(name string, args map[string]any, wantError bool) {
				t.Helper()
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
				if failed := err != nil || result.IsError; failed != wantError {
					t.Fatalf("%s failed=%v: %v %v", name, failed, result, err)
				}
			}
			if !consent {
				call("authrail_list_available_tools", map[string]any{}, true)
				call("authrail_get_available_tool", map[string]any{"routeId": routeID}, true)
				if delegated.Load() != 0 {
					t.Fatal("unconsented discovery reached API")
				}
				return
			}
			call("authrail_list_available_tools", map[string]any{"limit": 3, "cursor": cursorID}, false)
			call("authrail_get_available_tool", map[string]any{"routeId": routeID}, false)
			for _, args := range []map[string]any{{"limit": 0}, {"limit": 11}, {"cursor": "../../credentials"}, {"organisationId": "other-org"}, {"endpoint": "https://attacker.invalid"}} {
				call("authrail_list_available_tools", args, true)
			}
			call("authrail_get_available_tool", map[string]any{"routeId": "../../credentials"}, true)
			call("authrail_get_available_tool", map[string]any{"routeId": routeID, "credential": "unsafe"}, true)
			call("linear.issues.create", map[string]any{"title": "Not authorised by discovery"}, true)
			if delegated.Load() != 2 {
				t.Fatal("invalid input or unregistered execution reached API")
			}
			wrongIdentity.Store(true)
			call("authrail_get_available_tool", map[string]any{"routeId": routeID}, true)
			discoveryGranted.Store(false)
			call("authrail_get_available_tool", map[string]any{"routeId": routeID}, true)
			if delegated.Load() != 3 {
				t.Fatal("withdrawn discovery consent retained delegation authority")
			}
		})
	}
}
