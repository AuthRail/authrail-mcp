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

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(clone)
}

// We check that calls keep the signed-in user and workspace, including after revocation.
// This HTTP fixture tests the Go transport; the server integration suite tests real OAuth.
func TestConnectedToolsRecheckAuthorityAndIsolateIdentity(t *testing.T) {
	var revoked atomic.Bool
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/mcp/introspect":
			if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) {
				t.Error("missing service authentication")
			}
			var input struct{ Token string }
			_ = json.NewDecoder(r.Body).Decode(&input)
			if revoked.Load() || input.Token != strings.Repeat("u", 32) {
				_ = json.NewEncoder(w).Encode(map[string]bool{"active": false})
				return
			}
			_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user-1", SessionID: "session-1", OrganisationID: "org-1", ClientID: "client-1", Scopes: []string{Scope}, ExpiresAt: time.Now().Add(time.Minute)})
		case "/auth/mcp/delegations/exchange":
			if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) {
				t.Error("resource token used as service credential")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "api-delegation", "expiresAt": time.Now().Add(time.Minute)})
		case "/organisation/mcp/workspace":
			if r.Header.Get("Authorization") != "Bearer api-delegation" || r.Header.Get("X-Organisation-Id") != "org-1" {
				t.Error("incorrect API delegation or tenant binding")
			}
			calls.Add(1)
			_ = json.NewEncoder(w).Encode(Workspace{Organisation: NamedIdentity{ID: "org-1", Name: "Workspace"}, User: NamedIdentity{ID: "user-1", Name: "Reviewer"}, Permissions: []string{Scope}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
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
	client := mcp.NewClient(&mcp.Implementation{Name: "verification", Version: "1"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: resource, HTTPClient: &http.Client{Transport: bearerTransport{strings.Repeat("u", 32)}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 2 {
		t.Fatalf("tools: %v %v", tools, err)
	}
	for _, name := range []string{"authrail_get_connection", "authrail_get_workspace_summary"} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
		if err != nil || result.IsError {
			t.Fatalf("%s: %v %v", name, result, err)
		}
		encoded, _ := json.Marshal(result)
		if strings.Contains(string(encoded), strings.Repeat("u", 32)) || strings.Contains(string(encoded), "session-1") {
			t.Fatal("secret/session disclosed")
		}
	}
	invalid, invalidErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_get_workspace_summary", Arguments: map[string]any{"organisationId": "org-2"}})
	if invalidErr == nil && !invalid.IsError {
		t.Fatal("tool accepted an agent-supplied workspace")
	}
	if calls.Load() != 2 {
		t.Fatal("tools did not reach protected API")
	}
	revoked.Store(true)
	if _, err := session.ListTools(ctx, nil); err == nil {
		t.Fatal("revoked connection remained authorised")
	}
	for _, input := range []struct{ host, origin string }{{"attacker.invalid", ""}, {"", "https://attacker.invalid"}} {
		req, _ := http.NewRequest(http.MethodGet, endpoint.URL+"/health/live", nil)
		if input.host != "" {
			req.Host = input.host
		}
		req.Header.Set("Origin", input.origin)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 403 {
			t.Fatalf("unsafe host/origin accepted: %d", response.StatusCode)
		}
	}
}

// We reject redirects so credentials stay with AuthRail, and limit response size before decoding.
func TestBackendRejectsRedirectsAndOversizedResponses(t *testing.T) {
	var received atomic.Bool
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Store(true) }))
	defer sink.Close()
	for _, mode := range []string{"redirect", "oversize", "wrong-identity"} {
		t.Run(mode, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "redirect" {
					http.Redirect(w, r, sink.URL, 307)
					return
				}
				if mode == "oversize" {
					_, _ = w.Write([]byte(strings.Repeat("x", maxResponse+1)))
					return
				}
				if r.URL.Path == "/auth/mcp/delegations/exchange" {
					_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "delegation", "expiresAt": time.Now().Add(time.Minute)})
					return
				}
				_ = json.NewEncoder(w).Encode(Workspace{Organisation: NamedIdentity{ID: "wrong-tenant"}, User: NamedIdentity{ID: "wrong-user"}, Permissions: []string{Scope}})
			}))
			defer api.Close()
			b, err := NewBackend(Config{APIURL: api.URL, ResourceURL: "https://mcp.example.test/mcp", ServiceToken: strings.Repeat("s", 32), AllowLoopback: true})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "wrong-identity" {
				if _, err = b.Workspace(context.Background(), strings.Repeat("u", 32), "authrail_get_connection", Identity{UserID: "user", OrganisationID: "tenant"}); err == nil {
					t.Fatal("tenant mismatch accepted")
				}
			} else if _, err = b.Introspect(context.Background(), strings.Repeat("u", 32)); err == nil {
				t.Fatal("unsafe backend response accepted")
			}
		})
	}
	if received.Load() {
		t.Fatal("credential-bearing redirect followed")
	}
}
