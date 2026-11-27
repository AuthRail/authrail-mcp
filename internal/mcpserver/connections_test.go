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

// Retired collection setup must be absent and uncallable even under combined management consent.
func TestConnectionsPublishBusinessAndCustomAccountsWithoutCollectionTools(t *testing.T) {
	for _, scopes := range [][]string{
		{Scope, ConnectionsScope}, {Scope, ConnectionsManageScope},
		{Scope, ConnectionsScope, ConnectionsManageScope, PoliciesReadScope, PoliciesManageScope, OrganisationManageScope, AccountManageScope, ComplianceVerificationReadScope},
	} {
		t.Run(strings.Join(scopes, "+"), func(t *testing.T) {
			var unexpected atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/auth/mcp/introspect" {
					_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user", SessionID: "session", OrganisationID: "org", ClientID: "client", Scopes: scopes, ExpiresAt: time.Now().Add(time.Minute)})
					return
				}
				unexpected.Add(1)
				w.WriteHeader(http.StatusNotFound)
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
			session, err := mcp.NewClient(&mcp.Implementation{Name: "fixture", Version: "1"}, nil).Connect(ctx,
				&mcp.StreamableClientTransport{Endpoint: resource, HTTPClient: &http.Client{Transport: bearerTransport{strings.Repeat("u", 32)}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			info := session.InitializeResult().ServerInfo
			if info.Name != "authrail-mcp" || info.Title != "AuthRail" || info.WebsiteURL != "https://authrail.io" || len(info.Icons) != 1 || info.Icons[0].MIMEType != "image/png" || !strings.HasPrefix(info.Icons[0].Source, "https://") {
				t.Fatal("initialization omitted the AuthRail brand metadata")
			}
			if session.InitializeResult().Instructions == "" {
				t.Fatal("server guidance was not delivered during initialization")
			}
			inventory, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			for _, tool := range inventory.Tools {
				found[tool.Name] = true
			}
			for _, retired := range []string{"authrail_list_connection_providers", "authrail_manage_connection", "authrail_list_connections", "authrail_get_connection_status"} {
				if found[retired] {
					t.Fatalf("retired collection tool published: %s", retired)
				}
				result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: retired, Arguments: map[string]any{}})
				if callErr == nil && !result.IsError {
					t.Fatalf("retired collection tool remained callable: %s", retired)
				}
			}
			for _, name := range []string{"authrail_get_connection", "authrail_get_workspace_summary"} {
				if !found[name] {
					t.Fatalf("assistant identity tool removed: %s", name)
				}
			}
			if len(scopes) > 3 {
				for _, name := range []string{"authrail_list_business_providers", "authrail_manage_business_account", "authrail_list_custom_connections", "authrail_manage_custom_connection", "authrail_list_agent_connections", "authrail_manage_agent_connection", "authrail_create_policy_draft", "authrail_manage_organisation", "authrail_update_my_account_profile", "authrail_get_resource_history"} {
					if !found[name] {
						t.Fatalf("current account/governance function removed: %s", name)
					}
				}
			}
			if unexpected.Load() != 0 {
				t.Fatal("retired tool reached backend delegation or collection API")
			}
		})
	}
}
