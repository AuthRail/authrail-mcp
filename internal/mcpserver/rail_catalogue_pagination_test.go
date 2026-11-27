package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestConnectedActionCataloguePreservesContinuationBeyondForty(t *testing.T) {
	cursor := "00000000-0000-4000-8000-000000000040"
	reads := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/mcp/introspect":
			_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user", SessionID: "session", OrganisationID: "org", ClientID: "client", Scopes: []string{Scope, RuntimeDraftsScope}, ExpiresAt: time.Now().Add(time.Minute)})
		case "/auth/mcp/delegations/exchange":
			_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "delegated", "expiresAt": time.Now().Add(time.Minute)})
		case "/organisation/mcp/rails/actions":
			if r.Method != http.MethodGet || r.Header.Get("X-Organisation-Id") != "org" || r.Header.Get("Authorization") != "Bearer delegated" || r.URL.Query().Get("limit") != "40" {
				t.Error("wrong catalogue request authority or limit")
			}
			reads++
			start, end := 1, 40
			more := true
			var next any = cursor
			if r.URL.Query().Get("afterId") != "" {
				if r.URL.Query().Get("afterId") != cursor {
					t.Error("changed returned continuation")
				}
				start, end, more, next = 41, 44, false, nil
			}
			actions := []map[string]string{}
			for i := start; i <= end; i++ {
				actions = append(actions, map[string]string{"id": fmt.Sprintf("00000000-0000-4000-8000-%012d", i)})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": "org", "userId": "user", "data": map[string]any{"actions": actions, "hasMore": more, "nextAfterId": next}})
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
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
	session, err := mcp.NewClient(&mcp.Implementation{Name: "pagination-verification", Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: resource, HTTPClient: &http.Client{Transport: bearerTransport{strings.Repeat("u", 32)}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	total := 0
	for _, after := range []string{"", cursor} {
		arguments := map[string]any{"limit": 40}
		if after != "" {
			arguments["afterId"] = after
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_list_connected_actions", Arguments: arguments})
		if err != nil || result.IsError {
			t.Fatalf("catalogue failed: %v %v", result, err)
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			Actions     []map[string]string `json:"actions"`
			HasMore     bool                `json:"hasMore"`
			NextAfterID *string             `json:"nextAfterId"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			t.Fatal(err)
		}
		if after == "" && (!page.HasMore || page.NextAfterID == nil || *page.NextAfterID != cursor) {
			t.Fatal("lost first-page continuation")
		}
		if after != "" && (page.HasMore || page.NextAfterID != nil) {
			t.Fatal("lost terminal page")
		}
		total += len(page.Actions)
	}
	if total != 44 || reads != 2 {
		t.Fatalf("catalogue truncated: %d actions in %d reads", total, reads)
	}
}
