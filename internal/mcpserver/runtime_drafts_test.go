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

// Guard discovery must keep its consent, exact delegation and tenant binding.
// This verifies the MCP transport contract, not server-side OAuth or publication.
func TestGuardProtectionsUseConsentedDelegation(t *testing.T) {
	for _, scenario := range []string{"allowed", "missing-scope", "wrong-identity"} {
		t.Run(scenario, func(t *testing.T) {
			var reads atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/auth/mcp/introspect":
					scopes := []string{Scope}
					if scenario != "missing-scope" {
						scopes = append(scopes, RuntimeDraftsScope)
					}
					_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user-1", SessionID: "session-1", OrganisationID: "org-1", ClientID: "client-1", Scopes: scopes, ExpiresAt: time.Now().Add(time.Minute)})
				case "/auth/mcp/delegations/exchange":
					var input struct{ Token, Tool string }
					_ = json.NewDecoder(r.Body).Decode(&input)
					if r.Method != http.MethodPost || input.Token != strings.Repeat("u", 32) || input.Tool != "authrail_list_guard_protections" || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) {
						t.Error("incorrect named-tool delegation")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "guard-delegation", "expiresAt": time.Now().Add(time.Minute)})
				case "/organisation/mcp/rails/guard-protections":
					if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer guard-delegation" || r.Header.Get("X-Organisation-Id") != "org-1" || r.URL.RawQuery != "" {
						t.Error("incorrect Guard catalogue request")
					}
					reads.Add(1)
					organisation := "org-1"
					if scenario == "wrong-identity" {
						organisation = "org-2"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": organisation, "userId": "user-1", "data": map[string]any{"protections": []string{"block_credentials_v1"}}})
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
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
			client := mcp.NewClient(&mcp.Implementation{Name: "verification", Version: "1"}, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
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
				if tool.Name == "authrail_list_guard_protections" {
					found = true
					if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
						t.Fatal("Guard discovery is not identified as read-only")
					}
				}
			}
			if found != (scenario != "missing-scope") {
				t.Fatal("Guard discovery ignored consent")
			}
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_list_guard_protections", Arguments: map[string]any{}})
			if scenario == "allowed" {
				if err != nil || result.IsError || reads.Load() != 1 {
					t.Fatalf("Guard discovery failed: %v %v", result, err)
				}
				invalid, invalidErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_list_guard_protections", Arguments: map[string]any{"organisationId": "org-2"}})
				if invalidErr == nil && !invalid.IsError || reads.Load() != 1 {
					t.Fatal("Guard discovery accepted an agent-supplied tenant")
				}
			} else if err == nil && !result.IsError {
				t.Fatal("Guard discovery accepted missing consent or mismatched identity")
			}
		})
	}
}

// Named routes and optimistic revisions must survive translation without giving
// the agent authority to choose an organisation or a publication endpoint.
func TestDataDraftToolsUseBoundedNamedRoutes(t *testing.T) {
	const sourceID = "00000000-0000-4000-8000-000000000001"
	const versionID = "00000000-0000-4000-8000-000000000002"
	const railID = "00000000-0000-4000-8000-000000000003"
	const operationID = "00000000-0000-4000-8000-000000000004"
	cases := []struct {
		name, method, path string
		args               map[string]any
	}{
		{"authrail_get_data_source_options", http.MethodGet, "/organisation/mcp/rails/data-source-options", map[string]any{}},
		{"authrail_list_data_sources", http.MethodGet, "/organisation/mcp/rails/data-sources?afterId=" + sourceID + "&limit=10", map[string]any{"limit": 10, "afterId": sourceID}},
		{"authrail_get_data_source", http.MethodGet, "/organisation/mcp/rails/data-sources/" + sourceID, map[string]any{"sourceId": sourceID}},
		{"authrail_get_data_source_version", http.MethodGet, "/organisation/mcp/rails/data-sources/" + sourceID + "/versions/" + versionID, map[string]any{"sourceId": sourceID, "versionId": versionID}},
		{"authrail_create_data_source_draft", http.MethodPost, "/organisation/mcp/rails/data-source-drafts", map[string]any{"idempotencyKey": sourceID, "key": "customer-consent", "name": "Customer consent", "contract": map[string]any{"formatVersion": 4, "kind": "sdk", "accountBoundary": "account-1"}}},
		{"authrail_save_data_source_draft", http.MethodPatch, "/organisation/mcp/rails/data-sources/" + sourceID + "/drafts/" + versionID, map[string]any{"sourceId": sourceID, "versionId": versionID, "revision": 7, "name": "Consent", "contract": map[string]any{"formatVersion": 4, "kind": "sdk", "accountBoundary": "account-1"}}},
		{"authrail_create_data_rail_draft", http.MethodPost, "/organisation/mcp/rails/data-drafts", map[string]any{"idempotencyKey": sourceID, "name": "Check consent", "operationVersionId": operationID, "sourceContextVersionId": versionID}},
		{"authrail_save_data_rail_draft", http.MethodPatch, "/organisation/mcp/rails/" + railID + "/data-drafts/" + versionID, map[string]any{"railId": railID, "versionId": versionID, "revision": 7, "name": "Check consent", "content": map[string]any{"formatVersion": 2, "examples": []any{}}}},
		{"authrail_test_data_rail_draft", http.MethodPost, "/organisation/mcp/rails/" + railID + "/data-drafts/" + versionID + "/test", map[string]any{"railId": railID, "versionId": versionID, "revision": 7}},
	}
	for _, scope := range []bool{true, false} {
		t.Run(map[bool]string{true: "consented", false: "no-consent"}[scope], func(t *testing.T) {
			var calls atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/auth/mcp/introspect" {
					scopes := []string{Scope}
					if scope {
						scopes = append(scopes, RuntimeDraftsScope)
					}
					_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user-1", SessionID: "session-1", OrganisationID: "org-1", ClientID: "client-1", Scopes: scopes, ExpiresAt: time.Now().Add(time.Minute)})
					return
				}
				if r.URL.Path == "/auth/mcp/delegations/exchange" {
					var input struct{ Token, Tool string }
					_ = json.NewDecoder(r.Body).Decode(&input)
					found := false
					for _, item := range cases {
						found = found || item.name == input.Tool
					}
					if !scope || !found || input.Token != strings.Repeat("u", 32) {
						t.Error("unexpected Data tool delegation")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": input.Tool, "expiresAt": time.Now().Add(time.Minute)})
					return
				}
				tool := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				found := false
				for _, item := range cases {
					if item.name != tool {
						continue
					}
					found = true
					if r.Method != item.method || r.URL.RequestURI() != item.path || r.Header.Get("X-Organisation-Id") != "org-1" {
						t.Errorf("wrong Data route %s %s", r.Method, r.URL.RequestURI())
					}
					if r.Method == http.MethodPatch || tool == "authrail_test_data_rail_draft" {
						var body map[string]any
						_ = json.NewDecoder(r.Body).Decode(&body)
						if body["revision"] != float64(7) || body["sourceId"] != nil || body["railId"] != nil || body["versionId"] != nil {
							t.Error("revision or path IDs were not translated correctly")
						}
					}
				}
				if !found {
					t.Errorf("unexpected Data route %s", r.URL.Path)
					w.WriteHeader(404)
					return
				}
				calls.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": "org-1", "userId": "user-1", "data": map[string]any{"accepted": true}})
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
			listed, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range cases {
				found := false
				for _, tool := range listed.Tools {
					found = found || tool.Name == item.name
				}
				if found != scope {
					t.Fatalf("tool %s ignored consent", item.name)
				}
				if !scope {
					continue
				}
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: item.name, Arguments: item.args})
				if err != nil || result.IsError {
					t.Fatalf("%s failed: %v %v", item.name, result, err)
				}
			}
			expected := int32(0)
			if scope {
				expected = int32(len(cases))
			}
			if calls.Load() != expected {
				t.Fatal("unexpected backend calls")
			}
			if scope {
				for _, invalid := range []map[string]any{{"railId": railID, "versionId": versionID, "revision": 0}, {"railId": railID, "versionId": versionID, "revision": 7, "organisationId": "org-2"}} {
					result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_test_data_rail_draft", Arguments: invalid})
					if err == nil && !result.IsError {
						t.Fatal("invalid revision or agent tenant accepted")
					}
				}
				if calls.Load() != expected {
					t.Fatal("invalid input reached backend")
				}
			}
		})
	}
}
