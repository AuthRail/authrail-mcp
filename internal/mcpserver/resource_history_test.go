package mcpserver

import (
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestResourceHistoryConsentAndExactCursor(t *testing.T) {
	for _, scope := range []string{ConnectionsScope, ComplianceVerificationReadScope} {
		t.Run(scope, func(t *testing.T) {
			var calls atomic.Int32
			id := "10000000-0000-4000-8000-000000000001"
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/auth/mcp/introspect":
					_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user-1", SessionID: "session-1", OrganisationID: "org-1", ClientID: "client-1", Scopes: []string{Scope, scope}, ExpiresAt: time.Now().Add(time.Minute)})
				case "/auth/mcp/delegations/exchange":
					var body struct{ Tool string }
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body.Tool != "authrail_get_resource_history" {
						t.Error("wrong named delegation")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "history-read", "expiresAt": time.Now().Add(time.Minute)})
				case "/organisation/mcp/connections/" + id + "/resources/" + id + "/history":
					calls.Add(1)
					if r.Method != "GET" || r.Header.Get("X-Organisation-Id") != "org-1" || r.Header.Get("Authorization") != "Bearer history-read" || r.URL.Query().Get("cursor") != "page+2" || r.URL.Query().Get("kind") != "observations" || r.URL.Query().Get("limit") != "7" {
						t.Error("lost identity or cursor")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": "org-1", "userId": "user-1", "data": map[string]any{"records": []any{}, "pageInfo": map[string]any{"nextCursor": nil, "hasNextPage": false}}})
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
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
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			client := mcp.NewClient(&mcp.Implementation{Name: "history-test", Version: "1"}, nil)
			session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: resource, HTTPClient: &http.Client{Transport: bearerTransport{strings.Repeat("u", 32)}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			tools, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, tool := range tools.Tools {
				if tool.Name == "authrail_get_resource_history" {
					found = true
				}
			}
			if found != (scope == ComplianceVerificationReadScope) {
				t.Fatal("history consent surface mismatch")
			}
			if !found {
				return
			}
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_get_resource_history", Arguments: map[string]any{"connectionId": id, "resourceId": id, "kind": "observations", "cursor": "page+2", "limit": 7}})
			if err != nil || result.IsError {
				t.Fatalf("history read failed: %v %v", result, err)
			}
			result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_get_resource_history", Arguments: map[string]any{"connectionId": id, "resourceId": id, "kind": "private", "limit": 11}})
			if err == nil && !result.IsError {
				t.Fatal("invalid history accepted")
			}
			if calls.Load() != 1 {
				t.Fatal("invalid read reached domain")
			}
		})
	}
}
