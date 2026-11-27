// Package mcpserver connects MCP tools to the AuthRail API.
package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Scope permits connection identity and released integration guidance.
const Scope = "workspace.read"

// GovernanceScope permits governance reads, subject to the user’s domain permissions.
const GovernanceScope = "governance.read"

// ConnectionsScope permits business, Custom MCP and A2A account reads with the user’s permission.
const ConnectionsScope = "connections.read"
const maxResponse = 64 << 10

type traceContextKey struct{}

var tracePattern = regexp.MustCompile(`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`)
var errBackend = errors.New("AuthRail could not complete this request; reconnect or try again")
var errInvalidAccessToken = errors.New("The assistant access token is inactive or expired")

// Config holds the API and public MCP addresses supplied by the deployment.
// We require HTTPS unless local HTTP has been explicitly enabled.
type Config struct {
	APIURL                 string
	AuthorizationServerURL string
	ResourceURL            string
	ServiceToken           string
	AllowLoopback          bool
	TrustedAPIOrigin       string
}

type Backend struct {
	origin       string
	issuer       string
	resource     string
	serviceToken string
	client       *http.Client
}
type Identity struct {
	Active         bool      `json:"active"`
	UserID         string    `json:"userId"`
	SessionID      string    `json:"sessionId"`
	OrganisationID string    `json:"organisationId"`
	ClientID       string    `json:"clientId"`
	Scopes         []string  `json:"scopes"`
	ExpiresAt      time.Time `json:"expiresAt"`
}
type NamedIdentity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Workspace struct {
	MCPURL       string        `json:"mcpUrl,omitempty"`
	Organisation NamedIdentity `json:"organisation"`
	User         NamedIdentity `json:"user"`
	Permissions  []string      `json:"permissions"`
}

func NewBackend(config Config) (*Backend, error) {
	if config.APIURL == "" || config.ResourceURL == "" {
		return nil, errors.New("AuthRail API and MCP resource URLs are required")
	}
	if config.AuthorizationServerURL == "" {
		config.AuthorizationServerURL = strings.TrimRight(config.APIURL, "/") + "/auth"
	}
	for _, endpoint := range []struct {
		raw      string
		resource bool
	}{
		{config.APIURL, false}, {config.ResourceURL, true}, {config.AuthorizationServerURL, false},
	} {
		u, err := url.Parse(endpoint.raw)
		if err != nil || u.Host == "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" {
			return nil, errors.New("invalid AuthRail URL")
		}
		if endpoint.resource && u.Path != "" {
			return nil, errors.New("MCP resource URL must be an origin without a path or trailing slash")
		}
		if endpoint.raw == config.APIURL && u.Path != "" {
			return nil, errors.New("AuthRail API URL must be an origin without a path")
		}
		loopback := u.Hostname() == "127.0.0.1" || u.Hostname() == "::1" || u.Hostname() == "localhost"
		trustedAPI := endpoint.raw == config.APIURL && config.TrustedAPIOrigin != "" && config.TrustedAPIOrigin == config.APIURL && u.Scheme == "http"
		if u.Scheme != "https" && !(config.AllowLoopback && loopback && u.Scheme == "http") && !trustedAPI {
			return nil, errors.New("AuthRail URLs require HTTPS; HTTP loopback needs explicit opt-in")
		}
	}
	if len(config.ServiceToken) < 32 || strings.ContainsAny(config.ServiceToken, "\r\n\x00 ") {
		return nil, errors.New("a private MCP service credential is required")
	}
	return &Backend{origin: config.APIURL, issuer: config.AuthorizationServerURL, resource: config.ResourceURL, serviceToken: config.ServiceToken,
		client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (b *Backend) call(ctx context.Context, method, path, bearer, org string, input, output any, timeout ...time.Duration) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return errBackend
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, b.origin+path, body)
	if err != nil {
		return errBackend
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json")
	if parent, ok := ctx.Value(traceContextKey{}).(string); ok {
		req.Header.Set("traceparent", parent)
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if org != "" {
		req.Header.Set("X-Organisation-Id", org)
	}
	client := b.client
	if len(timeout) == 1 && timeout[0] > 0 && timeout[0] <= 25*time.Second {
		copy := *b.client
		copy.Timeout = timeout[0]
		client = &copy
	}
	res, err := client.Do(req)
	if err != nil {
		return errBackend
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusForbidden {
		return errors.New("Your connection or account does not have permission for this task")
	}
	if res.StatusCode == http.StatusNotFound {
		return errors.New("This record is unavailable in the connected workspace")
	}
	if res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusUnprocessableEntity {
		return errors.New("AuthRail did not accept these arguments; check the selected records and supplied values")
	}
	if res.StatusCode == http.StatusConflict {
		return errors.New("The record changed or is not ready for this task; read and review its latest revision before retrying")
	}
	if res.StatusCode == http.StatusServiceUnavailable && path == "/organisation/mcp/policy-rails/proposals" {
		// Only fixed domain codes cross this boundary; never forward provider errors or payloads.
		var failure struct {
			Code string `json:"code"`
		}
		if json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&failure) == nil {
			switch failure.Code {
			case "rail_proposal_invalid":
				return errors.New("The generated Rail rules did not pass validation. No draft was changed; review the exact action fields and retry the proposal")
			case "rail_proposal_unavailable":
				return errors.New("AuthRail's Rail proposal service is temporarily unavailable. Your saved draft is unchanged; retry the proposal without reconnecting")
			case "rail_proposal_input_limit":
				return errors.New("The selected Policy or action exceeds the proposal limits. Select fewer sections or fields; your saved draft is unchanged")
			}
		}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return errBackend
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse || json.Unmarshal(data, output) != nil {
		return errBackend
	}
	return nil
}

// Ready checks dependency health and exact resource/issuer agreement without a customer identity.
func (b *Backend) Ready(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, b.origin+"/health/ready", nil)
	if err != nil {
		return false
	}
	response, err := b.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	var configuration struct {
		Resource            string `json:"resource"`
		AuthorizationServer string `json:"authorizationServer"`
	}
	// A healthy API with different OAuth addresses cannot authorize this listener.
	// Keep the service credential on the fixed private API; never follow a redirect.
	if err := b.call(ctx, http.MethodGet, "/auth/mcp/configuration", b.serviceToken, "", nil, &configuration); err != nil {
		return false
	}
	return configuration.Resource == b.resource && configuration.AuthorizationServer == b.issuer
}

// Introspect asks AuthRail to validate the token for each request.
// We check again so revocation and membership changes take effect immediately.
func (b *Backend) Introspect(ctx context.Context, token string) (Identity, error) {
	var identity Identity
	if len(token) < 20 || len(token) > 8192 {
		return identity, errInvalidAccessToken
	}
	err := b.call(ctx, http.MethodPost, "/auth/mcp/introspect", b.serviceToken, "", map[string]string{"token": token}, &identity)
	if err != nil {
		return Identity{}, err
	}
	if !identity.Active || identity.UserID == "" || identity.SessionID == "" || identity.OrganisationID == "" || identity.ClientID == "" || !identity.ExpiresAt.After(time.Now()) {
		return Identity{}, errInvalidAccessToken
	}
	return identity, nil
}

// Workspace reads the account and organisation allowed by this connection.
// We obtain a separate API credential before calling the workspace endpoint.
func (b *Backend) Workspace(ctx context.Context, token, tool string, identity Identity) (Workspace, error) {
	if tool != "authrail_get_connection" && tool != "authrail_get_workspace_summary" {
		return Workspace{}, errBackend
	}
	var delegation struct {
		AccessToken string    `json:"accessToken"`
		ExpiresAt   time.Time `json:"expiresAt"`
	}
	err := b.call(ctx, http.MethodPost, "/auth/mcp/delegations/exchange", b.serviceToken, "", map[string]string{"token": token, "tool": tool}, &delegation)
	if err != nil || delegation.AccessToken == "" || !delegation.ExpiresAt.After(time.Now()) {
		return Workspace{}, errBackend
	}
	var result Workspace
	err = b.call(ctx, http.MethodGet, "/organisation/mcp/workspace", delegation.AccessToken, identity.OrganisationID, nil, &result)
	if err != nil || result.User.ID != identity.UserID || result.Organisation.ID != identity.OrganisationID {
		return Workspace{}, errBackend
	}
	if !sameScopes(result.Permissions, identity.Scopes) {
		return Workspace{}, errBackend
	}
	return result, nil
}

// We compare the API’s effective capabilities with the live introspection result.
func sameScopes(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, scope := range a {
		if !slices.Contains(b, scope) {
			return false
		}
	}
	return slices.Contains(a, Scope)
}

// AuthorizationMetadata supports clients that discover the issuer at the MCP origin.
// Fetch only our fixed private API path; never forward caller tokens or headers.
func (b *Backend) AuthorizationMetadata(ctx context.Context) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.origin+"/.well-known/oauth-authorization-server/auth", nil)
	if err != nil {
		return nil, errBackend
	}
	req.Header.Set("Accept", "application/json")
	response, err := b.client.Do(req)
	if err != nil {
		return nil, errBackend
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errBackend
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return nil, errBackend
	}
	var document map[string]any
	if json.Unmarshal(data, &document) != nil || document["issuer"] != b.issuer {
		return nil, errBackend
	}
	// Keep the configured issuer and exact owned OAuth endpoints, including during discovery fallback.
	for _, endpoint := range []struct{ key, path string }{
		{"authorization_endpoint", "/oauth2/authorize"}, {"token_endpoint", "/oauth2/token"},
		{"registration_endpoint", "/oauth2/register"}, {"introspection_endpoint", "/oauth2/introspect"},
		{"revocation_endpoint", "/oauth2/revoke"},
	} {
		value, present := document[endpoint.key]
		if !present && endpoint.key != "authorization_endpoint" && endpoint.key != "token_endpoint" {
			continue
		}
		if value != b.issuer+endpoint.path {
			return nil, errBackend
		}
	}
	return document, nil
}
