package protectedmcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	authrail "github.com/AuthRail/authrail-sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type managedFixture struct {
	settings       Settings
	mu             sync.Mutex
	effects        []string
	reject         bool
	missingSession bool
	provider       *mcp.Server
}

func managedServer(t *testing.T, reviewed ...*mcp.Tool) *managedFixture {
	t.Helper()
	f := &managedFixture{}
	business := &mcp.Tool{Name: "business", InputSchema: map[string]any{"type": "object"}}
	if len(reviewed) != 0 {
		business = reviewed[0]
	}
	pin, err := SchemaDigest(business)
	if err != nil {
		t.Fatal(err)
	}
	provider := mcp.NewServer(&mcp.Implementation{Name: "reviewed-server", Version: "1"}, nil)
	f.provider = provider
	provider.AddTool(business, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		f.mu.Lock()
		f.effects = append(f.effects, req.Extra.Header.Get("Authorization"))
		f.mu.Unlock()
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "reported"}}}, nil
	})
	provider.AddTool(&mcp.Tool{Name: "owner_identity", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		boundary := "account-fixed"
		if req.Extra.Header.Get("Authorization") == "Bearer other" {
			boundary = "another-account"
		}
		return &mcp.CallToolResult{StructuredContent: map[string]any{"account": boundary}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return provider }, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		reject := f.reject
		missing := f.missingSession
		f.mu.Unlock()
		if missing && r.Header.Get("Mcp-Session-Id") != "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if reject {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	tokenFile := filepath.Join(t.TempDir(), "private-token")
	f.settings = Settings{DownstreamURL: server.URL + "/mcp", DownstreamTokenFile: tokenFile, AllowLoopback: true, Bindings: []Binding{{DownstreamName: business.Name, SchemaSHA256: pin}}}
	f.token(t, "old")
	return f
}
func (f *managedFixture) token(t *testing.T, value string) {
	t.Helper()
	if err := os.WriteFile(f.settings.DownstreamTokenFile, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}
func ownerIdentity(ctx context.Context, session *mcp.ClientSession) (string, error) {
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "owner_identity", Arguments: map[string]any{}})
	if err != nil {
		return "", err
	}
	value, ok := result.StructuredContent.(map[string]any)
	if !ok {
		return "", errors.New("identity unavailable")
	}
	boundary, _ := value["account"].(string)
	return boundary, nil
}
func requireStatus(t *testing.T, err error, status DownstreamStatus) {
	t.Helper()
	var typed *DownstreamError
	if !errors.As(err, &typed) || typed.Status != status {
		t.Fatalf("expected %s, got %v", status, err)
	}
}
func refreshReady(ctx context.Context, m *ManagedDownstream) error {
	// A completed verifier releases its one resource slot asynchronously.
	for {
		err := m.Refresh(ctx)
		var typed *DownstreamError
		if !errors.As(err, &typed) || typed.Status != DownstreamBusy {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
		}
	}
}
func TestManagedDownstreamPinsOperationAndRefreshesOnlySameAccount(t *testing.T) {
	f := managedServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m, err := ConnectManagedDownstream(ctx, f.settings, "account-fixed", ownerIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	pinned, release, err := m.AcquireDownstream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.token(t, "new")
	requireStatus(t, m.Refresh(ctx), DownstreamBusy)
	if _, err = pinned.ListTools(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = pinned.CallTool(ctx, &mcp.CallToolParams{Name: "business", Arguments: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	release()
	release()
	if err = refreshReady(ctx, m); err != nil {
		t.Fatal(err)
	}
	if _, err = m.CallTool(ctx, &mcp.CallToolParams{Name: "business", Arguments: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	f.token(t, "other")
	requireStatus(t, refreshReady(ctx, m), IdentityChanged)
	if _, err = m.CallTool(ctx, &mcp.CallToolParams{Name: "business", Arguments: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.effects) != 3 || f.effects[0] != "Bearer old" || f.effects[1] != "Bearer new" || f.effects[2] != "Bearer new" {
		t.Fatalf("credential swap changed pinned call or retried a write: %v", f.effects)
	}
}
func TestManagedDownstreamBoundsIgnoredVerifierAndPreservesOldSession(t *testing.T) {
	f := managedServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls atomic.Int32
	entered := make(chan struct{})
	finish := make(chan struct{})
	exited := make(chan struct{})
	verifier := func(ctx context.Context, s *mcp.ClientSession) (string, error) {
		if calls.Add(1) > 1 {
			close(entered)
			<-finish
			defer close(exited)
		}
		return ownerIdentity(ctx, s)
	}
	m, err := ConnectManagedDownstream(ctx, f.settings, "account-fixed", verifier)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	// Wait for the initial trusted callback's resource slot to become free.
	for len(m.verification) != 0 {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	short, stop := context.WithTimeout(ctx, 50*time.Millisecond)
	defer stop()
	started := time.Now()
	requireStatus(t, m.Refresh(short), TransportUnknown)
	if time.Since(started) > time.Second {
		t.Fatal("ignored verifier blocked caller")
	}
	<-entered
	requireStatus(t, m.Refresh(ctx), DownstreamBusy)
	if _, err = m.CallTool(ctx, &mcp.CallToolParams{Name: "business", Arguments: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	close(finish)
	select {
	case <-exited:
	case <-ctx.Done():
		t.Fatal("late verifier did not finish")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.effects) != 1 {
		t.Fatal("refresh executed/retried business work")
	}
}
func TestManagedDownstreamRejectsCredentialsAndContractChanges(t *testing.T) {
	f := managedServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m, err := ConnectManagedDownstream(ctx, f.settings, "account-fixed", ownerIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	f.mu.Lock()
	f.reject = true
	f.mu.Unlock()
	// Opaque HTTP rejection is never labelled expired or revoked.
	requireStatus(t, refreshReady(ctx, m), CredentialsRejected)
	_, callErr := m.CallTool(ctx, &mcp.CallToolParams{Name: "business", Arguments: map[string]any{}})
	requireStatus(t, callErr, CredentialsRejected)
	f.mu.Lock()
	f.reject = false
	f.mu.Unlock()
	if err = os.Chmod(f.settings.DownstreamTokenFile, 0644); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, refreshReady(ctx, m), CredentialsUnavailable)
	if err = os.Chmod(f.settings.DownstreamTokenFile, 0600); err != nil {
		t.Fatal(err)
	}
	// A server that no longer matches reviewed contracts cannot replace a session.
	f.provider.AddTool(&mcp.Tool{Name: "business", InputSchema: map[string]any{"type": "object", "required": []string{"newField"}, "properties": map[string]any{"newField": map[string]any{"type": "string"}}}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	if err = refreshReady(ctx, m); err == nil {
		t.Fatal("changed tool contract accepted")
	}
	if _, err = m.ListTools(ctx, nil); err != nil {
		t.Fatal("failed candidate closed original session", err)
	}
}
func TestPrivateCustomerReaderRefreshesTokenWithoutRetargetingAccount(t *testing.T) {
	var seen []string
	var mu sync.Mutex
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		account := "fixed-account"
		if r.Header.Get("Authorization") == "Bearer other" {
			account = "other-account"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"accountId": account, "recordId": "invoice-1", "observedAt": time.Now().UTC().Format(time.RFC3339Nano), "data": map[string]any{"status": "open"}})
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "token")
	write := func(token string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(token), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("old")
	reader, err := privateCustomerAPIReader(authrail.CustomerAPIConfig{Endpoint: server.URL, SourceIdentity: "customer.billing", AccountBoundary: "fixed-account", Lookup: "invoice.read", HTTPClient: server.Client()}, file)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"old", "new"} {
		write(token)
		result, err := reader(context.Background(), "invoice-1")
		if err != nil || result.AccountBoundary != "fixed-account" {
			t.Fatalf("read failed after rotation: %v", err)
		}
	}
	write("other")
	if _, err = reader(context.Background(), "invoice-1"); err == nil {
		t.Fatal("new credential silently retargeted account")
	}
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	_, err = reader(context.Background(), "invoice-1")
	requireStatus(t, err, CredentialsUnavailable)
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 3 || seen[0] != "Bearer old" || seen[1] != "Bearer new" || seen[2] != "Bearer other" {
		t.Fatal("read did not reload credentials", seen)
	}
}

func TestConnectorLeasesBeforeResourceRecheckAndKeepsReplayAcrossRefresh(t *testing.T) {
	config, downstream, _ := fixture(t)
	f := managedServer(t, downstream.tool)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	managed, err := ConnectManagedDownstream(ctx, f.settings, "account-fixed", ownerIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer managed.Close()
	config.Downstream = managed
	lookups := 0
	config.ResolveResource = func(ctx context.Context, _ Binding, _ map[string]any) (string, error) {
		lookups++
		if lookups == 2 {
			f.token(t, "new")
			requireStatus(t, managed.Refresh(ctx), DownstreamBusy)
			managed.mu.Lock()
			active := managed.active
			managed.mu.Unlock()
			if active != 1 {
				t.Fatal("resource recheck did not hold exact session lease")
			}
		}
		return "invoice-1", nil
	}
	connector, err := New(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer connector.Close()
	result, err := connector.invoke(ctx, "pay", request())
	if err != nil || result.Status != "completed" {
		t.Fatalf("call failed: %+v %v", result, err)
	}
	if err = refreshReady(ctx, managed); err != nil {
		t.Fatal(err)
	}
	_, err = connector.invoke(ctx, "pay", request())
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.effects) != 1 || f.effects[0] != "Bearer old" {
		t.Fatalf("refresh lost retained execution or dispatched another write: %v", f.effects)
	}
}
func TestManagedDownstreamRequiresOwnerProofAndKeepsVerifierFacts(t *testing.T) {
	f := managedServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := ConnectManagedDownstream(ctx, f.settings, "account-fixed", nil); err == nil {
		t.Fatal("missing owner proof accepted")
	}
	for _, status := range []DownstreamStatus{CredentialExpired, CredentialRevoked} {
		_, err := ConnectManagedDownstream(ctx, f.settings, "account-fixed", func(context.Context, *mcp.ClientSession) (string, error) { return "", &DownstreamError{Status: status} })
		requireStatus(t, err, status)
	}
	f.token(t, "other")
	_, err := ConnectManagedDownstream(ctx, f.settings, "account-fixed", ownerIdentity)
	requireStatus(t, err, IdentityChanged)
}

func TestManagedDownstreamMissingSessionNeverRepeatsBusinessCall(t *testing.T) {
	f := managedServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m, err := ConnectManagedDownstream(ctx, f.settings, "account-fixed", ownerIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	f.mu.Lock()
	f.missingSession = true
	f.mu.Unlock()
	_, err = m.CallTool(ctx, &mcp.CallToolParams{Name: "business", Arguments: map[string]any{}})
	requireStatus(t, err, SessionUnavailable)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.effects) != 0 {
		t.Fatal("missing session replayed business call")
	}
}
