package mcpserver

import (
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"slices"
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

func TestRootResourceRejectsLegacyAndNoncanonicalURLs(t *testing.T) {
	for _, resource := range []string{"https://mcp.authrail.io/mcp", "https://mcp.authrail.io/", "https://mcp.authrail.io/path", "https://mcp.authrail.io?token=value", "https://mcp.authrail.io/#fragment"} {
		t.Run(resource, func(t *testing.T) {
			if _, err := NewBackend(Config{APIURL: "https://api.authrail.io", ResourceURL: resource, ServiceToken: strings.Repeat("s", 32)}); err == nil {
				t.Fatal("noncanonical public resource accepted")
			}
		})
	}
	if _, err := NewBackend(Config{APIURL: "https://api.authrail.io", ResourceURL: "https://mcp.authrail.io", ServiceToken: strings.Repeat("s", 32)}); err != nil {
		t.Fatal(err)
	}
}

func TestRootListenerPublishesExactMetadataAndRejectsPathAliases(t *testing.T) {
	backend, err := NewBackend(Config{APIURL: "https://api.authrail.io", ResourceURL: "https://mcp.authrail.io", ServiceToken: strings.Repeat("s", 32)})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(backend)
	metadataPath := "/.well-known/oauth-protected-resource"
	metadata := httptest.NewRecorder()
	handler.ServeHTTP(metadata, httptest.NewRequest(http.MethodGet, "https://mcp.authrail.io"+metadataPath, nil))
	var document struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
		ScopesSupported      []string `json:"scopes_supported"`
	}
	if metadata.Code != http.StatusOK || json.Unmarshal(metadata.Body.Bytes(), &document) != nil ||
		document.Resource != "https://mcp.authrail.io" || !slices.Equal(document.AuthorizationServers, []string{"https://api.authrail.io/auth"}) ||
		!slices.Contains(document.ScopesSupported, BusinessToolsReadScope) {
		t.Fatalf("incorrect root metadata: %d %s", metadata.Code, metadata.Body.String())
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, "https://mcp.authrail.io/", nil))
		if response.Code != http.StatusUnauthorized || !strings.Contains(response.Header().Get("WWW-Authenticate"), `resource_metadata="https://mcp.authrail.io`+metadataPath+`"`) {
			t.Fatalf("missing root authorization challenge: %d %s", response.Code, response.Header())
		}
	}
	for _, path := range []string{"/mcp", "/mcp/", metadataPath + "/mcp", "/unexpected", "//", "/./", "/%2f", "/%2e/", "/.well-known/%6fauth-protected-resource"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "https://mcp.authrail.io"+path, strings.NewReader(`{}`)))
		if response.Code != http.StatusNotFound || response.Header().Get("Location") != "" {
			t.Errorf("path %s accepted or redirected: %d %s", path, response.Code, response.Header())
		}
	}
	for _, origin := range []string{"null", "https://attacker.invalid", "https://mcp.authrail.io/"} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "https://mcp.authrail.io/", nil)
		request.Header.Set("Origin", origin)
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Errorf("unsafe origin accepted: %s", origin)
		}
	}
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
	if err != nil || len(tools.Tools) != 3 {
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
			b, err := NewBackend(Config{APIURL: api.URL, ResourceURL: "https://mcp.example.test", ServiceToken: strings.Repeat("s", 32), AllowLoopback: true})
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
