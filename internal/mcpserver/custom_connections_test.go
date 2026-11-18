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

func TestCustomConnectionsKeepReadConsentSeparateFromHumanHandoff(t *testing.T) {
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
					_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "custom-handoff", "expiresAt": time.Now().Add(time.Minute)})
				case "/organisation/mcp/connections/custom-mcp", "/organisation/mcp/connections/custom-mcp/" + connectionID, "/organisation/mcp/connections/custom-mcp/handoff":
					if r.Header.Get("Authorization") != "Bearer custom-handoff" || r.Header.Get("X-Organisation-Id") != "org" {
						t.Error("missing workspace delegation")
						w.WriteHeader(403)
						return
					}
					data := map[string]any{"records": []any{}, "pageInfo": map[string]any{"hasNextPage": false}}
					if strings.HasSuffix(r.URL.Path, "/handoff") {
						var body map[string]string
						if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 2 || body["action"] != "review" || body["connectionId"] != connectionID || r.Method != http.MethodPost {
							t.Error("unsafe handoff payload")
							w.WriteHeader(400)
							return
						}
						handoffs.Add(1)
						data = map[string]any{"handoff": map[string]any{"url": "https://app.example.test/connections/custom-mcp/" + connectionID, "requiresHuman": true}}
					} else if r.Method != http.MethodGet {
						t.Error("saved read mutated state")
					}
					if r.URL.Path == "/organisation/mcp/connections/custom-mcp" && (r.URL.Query().Get("limit") != "3" || r.URL.Query().Get("cursor") != "page+2") {
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
			if found["authrail_list_custom_connections"] != (scope == ConnectionsScope) || found["authrail_get_custom_connection"] != (scope == ConnectionsScope) || found["authrail_manage_custom_connection"] != (scope == ConnectionsManageScope) {
				t.Fatal("incorrect Custom connection consent boundary")
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
				call("authrail_list_custom_connections", map[string]any{"limit": 3, "cursor": "page+2"}, false)
				call("authrail_get_custom_connection", map[string]any{"connectionId": connectionID}, false)
				call("authrail_list_custom_connections", map[string]any{"limit": 11}, true)
				call("authrail_get_custom_connection", map[string]any{"connectionId": "../../credentials"}, true)
				call("authrail_manage_custom_connection", map[string]any{"action": "review", "connectionId": connectionID}, true)
				if delegated.Load() != 2 || handoffs.Load() != 0 {
					t.Fatal("invalid/read-only request reached management")
				}
			} else {
				call("authrail_manage_custom_connection", map[string]any{"action": "review", "connectionId": connectionID}, false)
				for _, args := range []map[string]any{{"action": "connect", "connectionId": connectionID}, {"action": "review"}, {"action": "review", "connectionId": connectionID, "endpoint": "https://unreviewed.example.test"}, {"action": "disconnect", "connectionId": connectionID, "credential": "secret"}} {
					call("authrail_manage_custom_connection", args, true)
				}
				if delegated.Load() != 1 || handoffs.Load() != 1 {
					t.Fatal("invalid handoff reached account API")
				}
			}
		})
	}
}
