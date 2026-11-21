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

func TestWorkspaceAccountRequiresOwnConsentedIdentity(t *testing.T) {
	for _, scenario := range []string{"allowed", "missing-scope", "wrong-identity"} {
		t.Run(scenario, func(t *testing.T) {
			var reads atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/auth/mcp/introspect":
					scopes := []string{Scope}
					if scenario != "missing-scope" {
						scopes = append(scopes, AccountReadScope)
					}
					_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user-1", SessionID: "session-1", OrganisationID: "org-1", ClientID: "client-1", Scopes: scopes, ExpiresAt: time.Now().Add(time.Minute)})
				case "/auth/mcp/delegations/exchange":
					var input struct{ Token, Tool string }
					_ = json.NewDecoder(r.Body).Decode(&input)
					if r.Method != http.MethodPost || input.Token != strings.Repeat("u", 32) || input.Tool != "authrail_get_my_account" || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) {
						t.Error("incorrect named-tool delegation")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "account-delegation", "expiresAt": time.Now().Add(time.Minute)})
				case "/organisation/mcp/account":
					if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer account-delegation" || r.Header.Get("X-Organisation-Id") != "org-1" || r.URL.RawQuery != "" {
						t.Error("incorrect Account catalogue request")
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
				if tool.Name == "authrail_get_my_account" {
					found = true
					if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
						t.Fatal("Account discovery is not identified as read-only")
					}
				}
			}
			if found != (scenario != "missing-scope") {
				t.Fatal("Account discovery ignored consent")
			}
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_get_my_account", Arguments: map[string]any{}})
			if scenario == "allowed" {
				if err != nil || result.IsError || reads.Load() != 1 {
					t.Fatalf("Account discovery failed: %v %v", result, err)
				}
				invalid, invalidErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_get_my_account", Arguments: map[string]any{"organisationId": "org-2"}})
				if invalidErr == nil && !invalid.IsError || reads.Load() != 1 {
					t.Fatal("Account discovery accepted an agent-supplied tenant")
				}
			} else if err == nil && !result.IsError {
				t.Fatal("Account discovery accepted missing consent or mismatched identity")
			}
		})
	}
}

// Discovery must distinguish each optional domain scope, including read-only consent.
func TestWorkspaceDiscoveryKeepsDomainConsentSeparate(t *testing.T) {
	groups := []struct{ scope, tool string }{
		{ComplianceConfigurationReadScope, "authrail_list_compliance_frameworks"}, {ComplianceConfigurationManageScope, "authrail_create_control_draft"},
		{ComplianceVerificationReadScope, "authrail_list_operational_findings"}, {ComplianceVerificationManageScope, "authrail_create_remediation_task"},
		{OrganisationReadScope, "authrail_get_organisation"}, {OrganisationManageScope, "authrail_manage_organisation"},
		{InventoryReadScope, "authrail_list_ai_inventory"}, {InventoryManageScope, "authrail_create_ai_inventory_item"},
		{ActivityReadScope, "authrail_list_activity_records"}, {ActivityRecordScope, "authrail_create_activity_record"},
		{PoliciesReadScope, "authrail_list_policies"}, {PoliciesManageScope, "authrail_create_policy_draft"},
		{AccountReadScope, "authrail_get_my_account"}, {AccountManageScope, "authrail_update_my_account_profile"},
		{NotificationsReadScope, "authrail_list_my_notifications"}, {NotificationsManageScope, "authrail_mark_notification_read"},
	}
	for _, group := range groups {
		t.Run(group.scope, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/auth/mcp/introspect" {
					t.Errorf("Discovery reached domain execution: %s", r.URL.Path)
					w.WriteHeader(500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user-1", SessionID: "session-1", OrganisationID: "org-1", ClientID: "client-1", Scopes: []string{Scope, group.scope}, ExpiresAt: time.Now().Add(time.Minute)})
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
			client := mcp.NewClient(&mcp.Implementation{Name: "consent-check", Version: "1"}, nil)
			session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: resource, HTTPClient: &http.Client{Transport: bearerTransport{strings.Repeat("u", 32)}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			listed, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			names := map[string]bool{}
			for _, tool := range listed.Tools {
				names[tool.Name] = true
			}
			for _, candidate := range groups {
				if names[candidate.tool] != (candidate.scope == group.scope) {
					t.Errorf("scope %s exposed %s=%v", group.scope, candidate.tool, names[candidate.tool])
				}
			}
		})
	}
}
