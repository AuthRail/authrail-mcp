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
		!slices.Contains(document.ScopesSupported, BusinessToolsReadScope) || !slices.Contains(document.ScopesSupported, "offline_access") {
		t.Fatalf("incorrect root metadata: %d %s", metadata.Code, metadata.Body.String())
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, "https://mcp.authrail.io/", nil))
		if response.Code != http.StatusUnauthorized || !strings.Contains(response.Header().Get("WWW-Authenticate"), `resource_metadata="https://mcp.authrail.io`+metadataPath+`"`) ||
			!strings.Contains(response.Header().Get("WWW-Authenticate"), `scope="workspace.read offline_access"`) {
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

func TestWorkspaceOnlyAccessTokenStillInitializesWithoutRenewalScope(t *testing.T) {
	var introspections atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/mcp/introspect" {
			t.Errorf("unexpected API path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		introspections.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user-1", SessionID: "session-1", OrganisationID: "org-1", ClientID: "client-1", Scopes: []string{Scope}, ExpiresAt: time.Now().Add(time.Minute)})
	}))
	defer api.Close()
	backend, err := NewBackend(Config{APIURL: api.URL, ResourceURL: "https://mcp.authrail.io", ServiceToken: strings.Repeat("s", 32), AllowLoopback: true, TrustedAPIOrigin: api.URL})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "https://mcp.authrail.io/", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"verification","version":"1"}}}`))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("t", 32))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()
	Handler(backend).ServeHTTP(response, request)
	var initialized struct {
		Result *struct {
			ServerInfo mcp.Implementation `json:"serverInfo"`
		} `json:"result"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &initialized) != nil || initialized.Result == nil || initialized.Result.ServerInfo.Name != "authrail-mcp" {
		t.Fatalf("workspace-only token failed initialization: %d %s", response.Code, response.Body.String())
	}
	if introspections.Load() != 1 || response.Header().Get("WWW-Authenticate") != "" {
		t.Fatal("valid token was challenged or skipped live authority verification")
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
	if err != nil || len(tools.Tools) != 6 {
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

// Dependency health alone is insufficient: tokens for another issuer/resource
// cannot authorize this listener. Configuration stays on the authenticated API.
func TestReadinessRequiresExactAuthenticatedOAuthConfiguration(t *testing.T) {
	for _, scenario := range []string{"aligned", "other-resource", "other-issuer", "missing", "unhealthy", "redirect"} {
		t.Run(scenario, func(t *testing.T) {
			var leaked atomic.Bool
			sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
			defer sink.Close()
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/health/ready":
					if r.Header.Get("Authorization") != "" {
						t.Error("health probe received service credential")
					}
					if scenario == "unhealthy" {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					w.WriteHeader(http.StatusOK)
				case "/auth/mcp/configuration":
					if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) {
						t.Error("configuration request missing private service credential")
					}
					if scenario == "redirect" {
						http.Redirect(w, r, sink.URL, http.StatusTemporaryRedirect)
						return
					}
					resource, issuer := "https://mcp.example.test", "https://api.example.test/auth"
					if scenario == "other-resource" {
						resource = "https://another.example.test"
					}
					if scenario == "other-issuer" {
						issuer = "https://another.example.test/auth"
					}
					if scenario == "missing" {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]string{"resource": resource, "authorizationServer": issuer})
				default:
					t.Errorf("unexpected readiness path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer api.Close()
			backend, err := NewBackend(Config{APIURL: api.URL, ResourceURL: "https://mcp.example.test", AuthorizationServerURL: "https://api.example.test/auth", ServiceToken: strings.Repeat("s", 32), AllowLoopback: true})
			if err != nil {
				t.Fatal(err)
			}
			if ready := backend.Ready(context.Background()); ready != (scenario == "aligned") {
				t.Fatalf("readiness=%v for %s", ready, scenario)
			}
			if leaked.Load() {
				t.Fatal("private service credential followed redirect")
			}
		})
	}
}

func TestMCPOriginAuthorizationDiscoveryKeepsTheConfiguredIssuer(t *testing.T) {
	issuer := "https://api.example.test/auth"
	mode := "valid"
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/.well-known/oauth-authorization-server/auth" || r.Method != http.MethodGet || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("discovery forwarded private context or changed the fixed API path")
		}
		w.Header().Set("Set-Cookie", "must-not-reach-client=private")
		if mode == "redirect" {
			w.Header().Set("Location", "https://elsewhere.example")
			w.WriteHeader(302)
			return
		}
		if mode == "oversize" {
			_, _ = w.Write([]byte(strings.Repeat("x", 65537)))
			return
		}
		doc := map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/oauth2/authorize", "token_endpoint": issuer + "/oauth2/token", "registration_endpoint": issuer + "/oauth2/register"}
		if mode == "wrong-issuer" {
			doc["issuer"] = "https://attacker.example/auth"
		}
		if mode == "wrong-endpoint" {
			doc["token_endpoint"] = "https://attacker.example/token"
		}
		_ = json.NewEncoder(w).Encode(doc)
	}))
	defer api.Close()
	backend, err := NewBackend(Config{APIURL: api.URL, AuthorizationServerURL: issuer, ResourceURL: "https://mcp.example.test", ServiceToken: strings.Repeat("s", 32), AllowLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(backend)
	for _, scenario := range []struct {
		mode string
		want int
	}{{"valid", 200}, {"wrong-issuer", 503}, {"wrong-endpoint", 503}, {"redirect", 503}, {"oversize", 503}} {
		mode = scenario.mode
		r := httptest.NewRequest(http.MethodGet, "https://mcp.example.test/.well-known/oauth-authorization-server", nil)
		r.Header.Set("Authorization", "Bearer must-not-forward")
		r.Header.Set("Cookie", "private=must-not-forward")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != scenario.want || w.Header().Get("Set-Cookie") != "" {
			t.Fatalf("%s discovery status/header: %d", mode, w.Code)
		}
		if scenario.want == 200 {
			var doc map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &doc)
			if doc["issuer"] != issuer {
				t.Fatal("issuer changed")
			}
		}
	}
	before := calls.Load()
	for _, r := range []*http.Request{httptest.NewRequest(http.MethodPost, "https://mcp.example.test/.well-known/oauth-authorization-server", nil), httptest.NewRequest(http.MethodGet, "https://untrusted.example/.well-known/oauth-authorization-server", nil)} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 405 && w.Code != 403 {
			t.Fatal("untrusted discovery accepted")
		}
	}
	if calls.Load() != before {
		t.Fatal("untrusted request reached backend")
	}
}

func TestTokenVerificationOutageDoesNotInvalidateAssistantConnection(t *testing.T) {
	for _, outage := range []bool{true, false} {
		t.Run(map[bool]string{true: "backend outage", false: "inactive token"}[outage], func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if outage {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"active":false}`))
			}))
			defer api.Close()
			backend, err := NewBackend(Config{APIURL: api.URL, ResourceURL: "https://mcp.authrail.io", ServiceToken: strings.Repeat("s", 32), AllowLoopback: true, TrustedAPIOrigin: api.URL})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "https://mcp.authrail.io/", strings.NewReader(`{}`))
			req.Header.Set("Authorization", "Bearer "+strings.Repeat("t", 32))
			res := httptest.NewRecorder()
			Handler(backend).ServeHTTP(res, req)
			expected := http.StatusUnauthorized
			if outage {
				expected = http.StatusInternalServerError
			}
			if res.Code != expected {
				t.Fatalf("got %d, want %d", res.Code, expected)
			}
		})
	}
}
