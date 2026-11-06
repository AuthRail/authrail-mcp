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
