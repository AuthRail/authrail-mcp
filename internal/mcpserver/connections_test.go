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

// Status consent cannot prepare account changes; management opens a human handoff only.
func TestConnectionManagementConsentAndSecureHandoff(t *testing.T) {
	for _, scope := range []string{ConnectionsScope, ConnectionsManageScope} {
		t.Run(scope, func(t *testing.T) {
			var calls atomic.Int32
			var providerReads atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/auth/mcp/introspect":
					_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user-1", SessionID: "session-1", OrganisationID: "org-1", ClientID: "client-1", Scopes: []string{Scope, scope}, ExpiresAt: time.Now().Add(time.Minute)})
				case "/auth/mcp/delegations/exchange":
					var input struct{ Token, Tool string }
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					if (input.Tool != "authrail_manage_connection" && input.Tool != "authrail_list_connection_providers") || input.Token != strings.Repeat("u", 32) || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) {
						t.Error("missing named-tool delegation")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "account-handoff", "expiresAt": time.Now().Add(time.Minute)})
				case "/organisation/mcp/connections/providers":
					if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer account-handoff" || r.Header.Get("X-Organisation-Id") != "org-1" || r.URL.Query().Get("limit") != "5" || r.URL.Query().Get("search") != "Linear & Notion" || r.URL.Query().Get("cursor") != "page+2" {
						t.Error("incorrect bounded provider query or authority")
					}
					providerReads.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": "org-1", "userId": "user-1", "data": map[string]any{"records": []any{}, "pageInfo": map[string]any{"hasNextPage": false, "nextCursor": nil}}})
				case "/organisation/mcp/connections/handoff":
					if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer account-handoff" || r.Header.Get("X-Organisation-Id") != "org-1" {
						t.Error("incorrect handoff authority")
					}
					var body map[string]string
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					if len(body) != 2 || body["action"] == "" {
						t.Errorf("unexpected handoff payload: %v", body)
					}
					calls.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": "org-1", "userId": "user-1", "data": map[string]any{"handoff": map[string]any{"url": "https://app.example.test/connections", "action": body["action"], "requiresHuman": true}}})
				default:
					t.Errorf("handoff reached executable/unrelated route: %s", r.URL.Path)
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
			found := false
			foundProviders := false
			for _, tool := range inventory.Tools {
				found = found || tool.Name == "authrail_manage_connection"
				foundProviders = foundProviders || tool.Name == "authrail_list_connection_providers"
			}
			if found != (scope == ConnectionsManageScope) || foundProviders != found {
				t.Fatal("status consent exposed management")
			}
			id := "00000000-0000-4000-8000-000000000001"
			if scope != ConnectionsManageScope {
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_manage_connection", Arguments: map[string]any{"action": "disconnect", "connectionId": id}})
				if err == nil && !result.IsError {
					t.Fatal("management accepted without consent")
				}
				if calls.Load() != 0 {
					t.Fatal("unconsented handoff reached API")
				}
				return
			}
			providers, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_list_connection_providers", Arguments: map[string]any{"search": "Linear & Notion", "cursor": "page+2"}})
			if err != nil || providers.IsError {
				t.Fatalf("provider discovery: %v %v", providers, err)
			}
			for _, arguments := range []map[string]any{{"limit": 11}, {"cursor": strings.Repeat("a", 501)}, {"search": strings.Repeat("s", 161)}} {
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_list_connection_providers", Arguments: arguments})
				if err == nil && !result.IsError {
					t.Fatal("unbounded provider query accepted")
				}
			}
			if providerReads.Load() != 1 {
				t.Fatal("invalid provider queries reached API")
			}
			for _, action := range []string{"connect", "reconnect", "disconnect"} {
				arguments := map[string]any{"action": action, "connectionId": id}
				if action == "connect" {
					arguments = map[string]any{"action": action, "definitionVersionId": id}
				}
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_manage_connection", Arguments: arguments})
				if err != nil || result.IsError {
					t.Fatalf("%s handoff: %v %v", action, result, err)
				}
			}
			for _, arguments := range []map[string]any{
				{"action": "connect", "connectionId": id},
				{"action": "reconnect", "definitionVersionId": id},
				{"action": "disconnect", "connectionId": id, "definitionVersionId": id},
				{"action": "disconnect", "connectionId": id, "credential": "secret"},
				{"action": "disconnect", "connectionId": id, "organisationId": "other"},
			} {
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_manage_connection", Arguments: arguments})
				if err == nil && !result.IsError {
					t.Fatalf("unsafe handoff accepted: %v", arguments)
				}
			}
			if calls.Load() != 3 {
				t.Fatal("invalid arguments reached account API")
			}
		})
	}
}
