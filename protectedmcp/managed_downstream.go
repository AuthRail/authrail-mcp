package protectedmcp

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DownstreamStatus string

const (
	CredentialsUnavailable DownstreamStatus = "credentials_unavailable"
	CredentialsRejected    DownstreamStatus = "credentials_rejected"
	SessionUnavailable     DownstreamStatus = "session_unavailable"
	TransportUnknown       DownstreamStatus = "transport_unknown"
	CredentialExpired      DownstreamStatus = "expired"
	CredentialRevoked      DownstreamStatus = "revoked"
	DownstreamBusy         DownstreamStatus = "busy"
	IdentityChanged        DownstreamStatus = "identity_changed"
)

type DownstreamError struct{ Status DownstreamStatus }

func (e *DownstreamError) Error() string { return "protected downstream: " + string(e.Status) }

// DownstreamIdentityVerifier is trusted owner code performing read-only authenticated
// account proof. Server metadata and agent-supplied identity are insufficient.
// Expired/revoked are accepted only as facts from this verifier, never inferred from HTTP prose.
type DownstreamIdentityVerifier func(context.Context, *mcp.ClientSession) (string, error)
type DownstreamOperationLeaser interface {
	AcquireDownstream(context.Context) (ToolClient, func(), error)
}

func leaseDownstream(ctx context.Context, client ToolClient) (ToolClient, func(), error) {
	if leased, ok := client.(DownstreamOperationLeaser); ok {
		return leased.AcquireDownstream(ctx)
	}
	return client, func() {}, nil
}

// ManagedDownstream refreshes only when the owner explicitly requests it. It never retries a tool call.
type ManagedDownstream struct {
	mu                        sync.Mutex
	verification              chan struct{}
	session                   *mcp.ClientSession
	settings                  Settings
	expectedBoundary          string
	verifier                  DownstreamIdentityVerifier
	serverName, serverVersion string
	active                    int
	closed                    bool
	refreshing                bool
	retiring                  bool
}

func ConnectManagedDownstream(ctx context.Context, settings Settings, expectedBoundary string, verifier DownstreamIdentityVerifier) (*ManagedDownstream, error) {
	if verifier == nil || strings.TrimSpace(expectedBoundary) != expectedBoundary || expectedBoundary == "" || len(expectedBoundary) > 256 || strings.ContainsAny(expectedBoundary, "\r\n") || len(settings.Bindings) == 0 || len(settings.Bindings) > 64 {
		return nil, errors.New("managed downstream requires a fixed account witness, reviewed bindings and trusted identity verifier")
	}
	// Keep all routing and contract pins independent from later caller-owned configuration changes.
	settings.Bindings = append([]Binding(nil), settings.Bindings...)
	value := &ManagedDownstream{settings: settings, expectedBoundary: expectedBoundary, verifier: verifier, verification: make(chan struct{}, 1)}
	session, err := value.candidate(ctx)
	if err != nil {
		return nil, err
	}
	value.session = session
	info := session.InitializeResult().ServerInfo
	value.serverName, value.serverVersion = info.Name, info.Version
	return value, nil
}
func (m *ManagedDownstream) candidate(parent context.Context) (*mcp.ClientSession, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	select {
	case m.verification <- struct{}{}:
	default:
		return nil, &DownstreamError{Status: DownstreamBusy}
	}
	candidate, err := ConnectDownstream(ctx, m.settings)
	if err != nil {
		<-m.verification
		return nil, err
	}
	accepted := false
	candidateDisposed := make(chan struct{})
	defer func() {
		if !accepted {
			go func() { _ = candidate.Close(); close(candidateDisposed) }()
		} else {
			close(candidateDisposed)
		}
	}()
	type identityResult struct {
		boundary string
		err      error
	}
	done := make(chan identityResult, 1)
	candidateFinished := make(chan struct{})
	defer close(candidateFinished)
	go func() {
		// Keep at most one verifier alive, including one that ignores cancellation.
		defer func() { <-candidateFinished; <-candidateDisposed; <-m.verification }()
		defer func() {
			if recover() != nil {
				done <- identityResult{err: &DownstreamError{Status: IdentityChanged}}
			}
		}()
		boundary, err := m.verifier(ctx, candidate)
		done <- identityResult{boundary, err}
	}()
	var identity identityResult
	select {
	case <-ctx.Done():
		return nil, &DownstreamError{Status: TransportUnknown}
	case identity = <-done:
	}
	if ctx.Err() != nil {
		return nil, &DownstreamError{Status: TransportUnknown}
	}
	if identity.err != nil {
		var known *DownstreamError
		if errors.As(identity.err, &known) && (known.Status == CredentialExpired || known.Status == CredentialRevoked || known.Status == CredentialsRejected || known.Status == CredentialsUnavailable || known.Status == TransportUnknown || known.Status == SessionUnavailable) {
			return nil, known
		}
		return nil, &DownstreamError{Status: IdentityChanged}
	}
	if identity.boundary != m.expectedBoundary {
		return nil, &DownstreamError{Status: IdentityChanged}
	}
	info := candidate.InitializeResult().ServerInfo
	if info == nil || info.Name == "" || info.Version == "" || (m.serverName != "" && (info.Name != m.serverName || info.Version != m.serverVersion)) {
		return nil, &DownstreamError{Status: IdentityChanged}
	}
	tools, err := discoverTools(ctx, candidate)
	if err != nil {
		return nil, &DownstreamError{Status: TransportUnknown}
	}
	for _, binding := range m.settings.Bindings {
		actual := tools[binding.DownstreamName]
		if actual == nil {
			return nil, errors.New("reviewed downstream tool is unavailable")
		}
		pin, err := SchemaDigest(actual)
		if err != nil || pin != binding.SchemaSHA256 {
			return nil, errors.New("reviewed downstream contract changed")
		}
	}
	if ctx.Err() != nil {
		return nil, &DownstreamError{Status: TransportUnknown}
	}
	accepted = true
	return candidate, nil
}
func (m *ManagedDownstream) AcquireDownstream(ctx context.Context) (ToolClient, func(), error) {
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	if !m.mu.TryLock() {
		return nil, nil, &DownstreamError{Status: DownstreamBusy}
	}
	if m.closed || m.session == nil {
		m.mu.Unlock()
		return nil, nil, &DownstreamError{Status: SessionUnavailable}
	}
	if m.active >= 64 {
		m.mu.Unlock()
		return nil, nil, &DownstreamError{Status: DownstreamBusy}
	}
	session := m.session
	m.active++
	m.mu.Unlock()
	var once sync.Once
	return session, func() { once.Do(func() { m.mu.Lock(); m.active--; m.mu.Unlock() }) }, nil
}
func (m *ManagedDownstream) ListTools(ctx context.Context, p *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	client, release, err := m.AcquireDownstream(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return client.ListTools(ctx, p)
}
func (m *ManagedDownstream) CallTool(ctx context.Context, p *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	client, release, err := m.AcquireDownstream(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return client.CallTool(ctx, p)
}
func (m *ManagedDownstream) Refresh(ctx context.Context) error {
	if !m.mu.TryLock() {
		return &DownstreamError{Status: DownstreamBusy}
	}
	busy, closed := m.active != 0 || m.refreshing || m.retiring, m.closed
	if !busy && !closed {
		m.refreshing = true
	}
	m.mu.Unlock()
	if closed {
		return &DownstreamError{Status: SessionUnavailable}
	}
	if busy {
		return &DownstreamError{Status: DownstreamBusy}
	}
	defer func() { m.mu.Lock(); m.refreshing = false; m.mu.Unlock() }()
	candidate, err := m.candidate(ctx)
	if err != nil {
		return err
	}
	if !m.mu.TryLock() {
		m.retire(candidate)
		return &DownstreamError{Status: DownstreamBusy}
	}
	if ctx.Err() != nil || m.closed || m.active != 0 {
		m.mu.Unlock()
		m.retire(candidate)
		return &DownstreamError{Status: DownstreamBusy}
	}
	old := m.session
	m.session = candidate
	m.mu.Unlock()
	m.retire(old)
	return nil
}

// Keep at most one superseded/candidate session awaiting the SDK's bounded close.
func (m *ManagedDownstream) retire(session *mcp.ClientSession) {
	m.mu.Lock()
	m.retiring = true
	m.mu.Unlock()
	go func() {
		_ = session.Close()
		m.mu.Lock()
		m.retiring = false
		m.mu.Unlock()
	}()
}
func (m *ManagedDownstream) Close() error {
	if !m.mu.TryLock() {
		return &DownstreamError{Status: DownstreamBusy}
	}
	if m.active != 0 {
		m.mu.Unlock()
		return &DownstreamError{Status: DownstreamBusy}
	}
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	session := m.session
	m.mu.Unlock()
	return session.Close()
}
