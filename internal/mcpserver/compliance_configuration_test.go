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

func TestComplianceConfigurationUsesNamedAuthority(t *testing.T) {
	for _, scenario := range []string{"allowed", "missing-scope", "wrong-identity"} {
		t.Run(scenario, func(t *testing.T) {
			var reads atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/auth/mcp/introspect":
					scopes := []string{Scope}
					if scenario != "missing-scope" {
						scopes = append(scopes, ComplianceConfigurationReadScope)
					}
					_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user-1", SessionID: "session-1", OrganisationID: "org-1", ClientID: "client-1", Scopes: scopes, ExpiresAt: time.Now().Add(time.Minute)})
				case "/auth/mcp/delegations/exchange":
					var input struct{ Token, Tool string }
					_ = json.NewDecoder(r.Body).Decode(&input)
					if r.Method != http.MethodPost || input.Token != strings.Repeat("u", 32) || input.Tool != "authrail_get_control_draft" || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) {
						t.Error("incorrect named-tool delegation")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "account-delegation", "expiresAt": time.Now().Add(time.Minute)})
				case "/organisation/mcp/compliance-configuration/controls/00000000-0000-4000-8000-000000000025/draft":
					if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer account-delegation" || r.Header.Get("X-Organisation-Id") != "org-1" || r.URL.RawQuery != "" {
						t.Error("incorrect Configuration catalogue request")
					}
					reads.Add(1)
					organisation := "org-1"
					if scenario == "wrong-identity" {
						organisation = "org-2"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": organisation, "userId": "user-1", "data": map[string]any{"account": map[string]string{"name": "Current person"}}})
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
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
				if tool.Name == "authrail_get_control_authoring" || tool.Name == "authrail_start_control_authoring" {
					t.Fatal("Policy-derived authoring discovered without Policy content consent")
				}
				if tool.Name == "authrail_get_control_draft" {
					found = true
					if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
						t.Fatal("Configuration discovery is not identified as read-only")
					}
				}
			}
			if found != (scenario != "missing-scope") {
				t.Fatal("Configuration discovery ignored consent")
			}
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_get_control_draft", Arguments: map[string]any{"controlId": "00000000-0000-4000-8000-000000000025"}})
			if scenario == "allowed" {
				if err != nil || result.IsError || reads.Load() != 1 {
					t.Fatalf("Configuration discovery failed: %v %v", result, err)
				}
				malformed, malformedErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_get_control_draft", Arguments: map[string]any{"controlId": "../another-tenant"}})
				if malformedErr == nil && !malformed.IsError || reads.Load() != 1 {
					t.Fatal("Malformed exact Control reference reached backend")
				}
				invalid, invalidErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_get_control_draft", Arguments: map[string]any{"organisationId": "org-2"}})
				if invalidErr == nil && !invalid.IsError || reads.Load() != 1 {
					t.Fatal("Configuration discovery accepted an agent-supplied tenant")
				}
			} else if err == nil && !result.IsError {
				t.Fatal("Configuration discovery accepted missing consent or mismatched identity")
			}
		})
	}
}
