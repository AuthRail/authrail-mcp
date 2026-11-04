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

// ConnectionsScope permits saved Connection status reads with the user’s permission.
const ConnectionsScope = "connections.read"
const maxResponse = 64 << 10

type traceContextKey struct{}

var tracePattern = regexp.MustCompile(`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`)
var errBackend = errors.New("AuthRail could not complete this request; reconnect or try again")

// Config holds the API and public MCP addresses supplied by the deployment.
// We require HTTPS unless local HTTP has been explicitly enabled.
type Config struct {
	APIURL                 string
	AuthorizationServerURL string
	ResourceURL            string
	ServiceToken           string
	AllowLoopback          bool
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
	for _, endpoint := range []struct{ raw, path string }{
		{config.APIURL, ""}, {config.ResourceURL, "/mcp"}, {config.AuthorizationServerURL, ""},
	} {
		u, err := url.Parse(endpoint.raw)
		if err != nil || u.Host == "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" {
			return nil, errors.New("invalid AuthRail URL")
		}
		if endpoint.path != "" && u.Path != endpoint.path {
			return nil, errors.New("MCP resource URL must end with /mcp")
		}
		if endpoint.raw == config.APIURL && u.Path != "" {
			return nil, errors.New("AuthRail API URL must be an origin without a path")
		}
		loopback := u.Hostname() == "127.0.0.1" || u.Hostname() == "::1" || u.Hostname() == "localhost"
		if u.Scheme != "https" && !(config.AllowLoopback && loopback && u.Scheme == "http") {
			return nil, errors.New("AuthRail URLs require HTTPS; HTTP loopback needs explicit opt-in")
		}
	}
	if len(config.ServiceToken) < 32 || strings.ContainsAny(config.ServiceToken, "\r\n\x00 ") {
		return nil, errors.New("a private MCP service credential is required")
	}
	return &Backend{origin: config.APIURL, issuer: config.AuthorizationServerURL, resource: config.ResourceURL, serviceToken: config.ServiceToken,
		client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (b *Backend) call(ctx context.Context, method, path, bearer, org string, input, output any) error {
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
	res, err := b.client.Do(req)
	if err != nil {
		return errBackend
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusForbidden {
		return errors.New("Your connection or account does not have permission to read this information")
	}
	if res.StatusCode == http.StatusNotFound {
		return errors.New("This record is unavailable in the connected workspace")
	}
	if res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusUnprocessableEntity {
		return errors.New("AuthRail did not accept these arguments; check the record IDs or restart pagination")
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

// Introspect asks AuthRail to validate the token for each request.
// We check again so revocation and membership changes take effect immediately.
func (b *Backend) Introspect(ctx context.Context, token string) (Identity, error) {
	var identity Identity
	if len(token) < 20 || len(token) > 8192 {
		return identity, errBackend
	}
	err := b.call(ctx, http.MethodPost, "/auth/mcp/introspect", b.serviceToken, "", map[string]string{"token": token}, &identity)
	if err != nil {
		return Identity{}, err
	}
	if !identity.Active || identity.UserID == "" || identity.SessionID == "" || identity.OrganisationID == "" || identity.ClientID == "" || !identity.ExpiresAt.After(time.Now()) {
		return Identity{}, errBackend
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
