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

// The actual MCP listener must retain named delegation, scope ceilings and exact
// revisions while keeping human decisions/file capabilities out of typed tools.
func TestInventoryActivityNamedWorkflows(t *testing.T) {
	const item = "00000000-0000-4000-8000-000000000001"
	const version = "00000000-0000-4000-8000-000000000002"
	const key = "00000000-0000-4000-8000-000000000003"
	inventory := map[string]any{"name": "Support agent", "providers": []any{}, "purpose": "Prepare replies", "dataClassifications": []any{}, "humanOversight": "Manager review", "departmentIds": []any{}, "teamIds": []any{}}
	activity := map[string]any{"inventoryItemId": item, "inventoryRevision": 7, "actionCategory": "automation", "occurredAt": "2026-11-19T10:00:00Z", "action": "Prepare reply", "dataClassification": "Internal", "outcomeSummary": "Prepared for review", "policyVersionIds": []any{}, "evidenceReferences": []any{}}
	cases := []struct {
		name, scope, method, path string
		args                      map[string]any
	}{
		{"authrail_list_ai_inventory", InventoryReadScope, http.MethodGet, "/organisation/mcp/inventory?limit=10", map[string]any{"limit": 10}},
		{"authrail_get_ai_inventory_item", InventoryReadScope, http.MethodGet, "/organisation/mcp/inventory/" + item, map[string]any{"itemId": item}},
		{"authrail_get_ai_inventory_history", InventoryReadScope, http.MethodGet, "/organisation/mcp/inventory/" + item + "/history?limit=10", map[string]any{"itemId": item, "limit": 10}},
		{"authrail_get_ai_inventory_revision", InventoryReadScope, http.MethodGet, "/organisation/mcp/inventory/" + item + "/revisions/" + version + "?limit=10&section=providers", map[string]any{"itemId": item, "revisionId": version, "section": "providers", "limit": 10}},
		{"authrail_create_ai_inventory_item", InventoryManageScope, http.MethodPost, "/organisation/mcp/inventory", map[string]any{"idempotencyKey": key, "fields": inventory}},
		{"authrail_update_ai_inventory_item", InventoryManageScope, http.MethodPatch, "/organisation/mcp/inventory/" + item, map[string]any{"itemId": item, "idempotencyKey": key, "revision": 7, "fields": inventory}},
		{"authrail_change_ai_inventory_lifecycle", InventoryManageScope, http.MethodPost, "/organisation/mcp/inventory/" + item + "/lifecycle", map[string]any{"itemId": item, "idempotencyKey": key, "revision": 7, "currentLifecycle": "active", "lifecycle": "inactive"}},
		{"authrail_review_ai_inventory_item", InventoryManageScope, http.MethodPost, "/organisation/mcp/inventory/" + item + "/review", map[string]any{"itemId": item, "revision": 7, "action": "confirm"}},
		{"authrail_list_activity_records", ActivityReadScope, http.MethodGet, "/organisation/mcp/activity?limit=3", map[string]any{"limit": 3}},
		{"authrail_list_activity_records", ActivityReadScope, http.MethodGet, "/organisation/mcp/activity?limit=3&source=assistant", map[string]any{"limit": 3, "source": "assistant"}},
		{"authrail_get_activity_record", ActivityReadScope, http.MethodGet, "/organisation/mcp/activity/" + item, map[string]any{"activityId": item}},
		{"authrail_get_activity_revision", ActivityReadScope, http.MethodGet, "/organisation/mcp/activity/" + item + "/revisions/" + version + "?", map[string]any{"activityId": item, "revisionId": version}},
		{"authrail_get_activity_history", ActivityReadScope, http.MethodGet, "/organisation/mcp/activity/" + item + "/history?limit=10", map[string]any{"activityId": item, "limit": 10}},
		{"authrail_get_activity_reviews", ActivityReadScope, http.MethodGet, "/organisation/mcp/activity/" + item + "/revisions/" + version + "/reviews?limit=4", map[string]any{"activityId": item, "revisionId": version, "limit": 4}},
		{"authrail_list_activity_categories", ActivityReadScope, http.MethodGet, "/organisation/mcp/activity/categories?limit=10", map[string]any{"limit": 10}},
		{"authrail_list_activity_classifications", ActivityReadScope, http.MethodGet, "/organisation/mcp/activity/classifications?limit=10", map[string]any{"limit": 10}},
		{"authrail_get_activity_attestation", ActivityReadScope, http.MethodGet, "/organisation/mcp/activity/" + item + "/attestation?revisionId=" + version, map[string]any{"activityId": item, "revisionId": version}},
		{"authrail_verify_activity_attestation", ActivityReadScope, http.MethodPost, "/organisation/mcp/activity/" + item + "/attestation/verify", map[string]any{"activityId": item, "revisionId": version}},
		{"authrail_download_activity_attestation", ActivityReadScope, http.MethodPost, "/organisation/mcp/activity/" + item + "/attestation/download", map[string]any{"activityId": item, "revisionId": version}},
		{"authrail_create_activity_record", ActivityRecordScope, http.MethodPost, "/organisation/mcp/activity", map[string]any{"idempotencyKey": key, "fields": activity}},
		{"authrail_correct_activity_record", ActivityRecordScope, http.MethodPost, "/organisation/mcp/activity/" + item + "/corrections", map[string]any{"activityId": item, "expectedRevisionId": version, "idempotencyKey": key, "fields": activity}},
		{"authrail_review_activity_record", ActivityRecordScope, http.MethodPost, "/organisation/mcp/activity/" + item + "/review", map[string]any{"activityId": item, "expectedRevisionId": version}},
		{"authrail_import_activity_records", ActivityRecordScope, http.MethodPost, "/organisation/mcp/activity/imports/handoff", map[string]any{}},
		{"authrail_get_activity_import", ActivityRecordScope, http.MethodGet, "/organisation/mcp/activity/imports/" + item, map[string]any{"importId": item}},
	}
	for _, scope := range []string{InventoryReadScope, InventoryManageScope, ActivityReadScope, ActivityRecordScope} {
		t.Run(scope, func(t *testing.T) {
			var calls atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/auth/mcp/introspect" {
					_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user-1", OrganisationID: "org-1", ClientID: "client-1", SessionID: "session-1", Scopes: []string{Scope, scope}, ExpiresAt: time.Now().Add(time.Minute)})
					return
				}
				if r.URL.Path == "/auth/mcp/delegations/exchange" {
					var body struct{ Token, Tool string }
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body.Token != strings.Repeat("u", 32) {
						t.Error("wrong subject token")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": body.Tool, "expiresAt": time.Now().Add(time.Minute)})
					return
				}
				tool := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				found := false
				for _, entry := range cases {
					if entry.name != tool || entry.path != r.URL.RequestURI() {
						continue
					}
					found = true
					if entry.scope != scope || r.Method != entry.method || r.URL.RequestURI() != entry.path || r.Header.Get("X-Organisation-Id") != "org-1" {
						t.Errorf("wrong named route: %s %s", r.Method, r.URL.RequestURI())
					}
					if r.Method != http.MethodGet {
						var body map[string]any
						_ = json.NewDecoder(r.Body).Decode(&body)
						if body["itemId"] != nil || body["activityId"] != nil || body["fields"] != nil || body["organisationId"] != nil || body["userId"] != nil {
							t.Error("path/authority fields were forwarded in mutation body")
						}
						if tool == "authrail_create_ai_inventory_item" && body["name"] != "Support agent" {
							t.Error("typed fields were not mapped")
						}
						if tool == "authrail_correct_activity_record" && body["expectedRevisionId"] != version {
							t.Error("exact correction revision lost")
						}
					}
				}
				if !found {
					t.Errorf("unexpected route %s", r.URL.Path)
					w.WriteHeader(404)
					return
				}
				calls.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": "org-1", "userId": "user-1", "data": map[string]any{"accepted": true}})
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
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			client := mcp.NewClient(&mcp.Implementation{Name: "inventory-verification", Version: "1"}, nil)
			session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: resource, HTTPClient: &http.Client{Transport: bearerTransport{strings.Repeat("u", 32)}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			listed, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			expected := int32(0)
			for _, entry := range cases {
				found := false
				for _, tool := range listed.Tools {
					found = found || tool.Name == entry.name
				}
				if found != (entry.scope == scope) {
					t.Fatalf("%s ignored scope discovery", entry.name)
				}
				if entry.scope != scope {
					continue
				}
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: entry.name, Arguments: entry.args})
				if err != nil || result.IsError {
					t.Fatalf("%s failed: %v %v", entry.name, result, err)
				}
				expected++
			}
			if calls.Load() != expected {
				t.Fatal("wrong number of named calls")
			}
			invalidName := "authrail_get_activity_record"
			args := map[string]any{"activityId": item, "organisationId": "other-org"}
			if scope == InventoryReadScope {
				invalidName = "authrail_get_ai_inventory_item"
				args = map[string]any{"itemId": item, "organisationId": "other-org"}
			}
			if scope == InventoryManageScope {
				invalidName = "authrail_review_ai_inventory_item"
				args = map[string]any{"itemId": item, "revision": 7, "action": "confirm", "confirmed": true}
			}
			if scope == ActivityRecordScope {
				invalidName = "authrail_review_activity_record"
				args = map[string]any{"activityId": item, "expectedRevisionId": version, "decision": "reviewed"}
			}
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: invalidName, Arguments: args})
			if err == nil && !result.IsError {
				t.Fatal("agent authority/human decision accepted")
			}
			if calls.Load() != expected {
				t.Fatal("invalid input reached domain")
			}
		})
	}
}
