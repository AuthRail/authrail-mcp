package protectedmcp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	authrail "github.com/AuthRail/authrail-sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const actionID = "11111111-1111-4111-8111-111111111111"
const approvalID = "22222222-2222-4222-8222-222222222222"
const permitID = "33333333-3333-4333-8333-333333333333"
const operationID = "66666666-6666-4666-8666-666666666666"

type downstreamFixture struct {
	mu      sync.Mutex
	effect  int
	unknown bool
	tool    *mcp.Tool
	seen    map[string]any
}

func (d *downstreamFixture) ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return &mcp.ListToolsResult{Tools: []*mcp.Tool{d.tool}}, nil
}
func (d *downstreamFixture) CallTool(_ context.Context, input *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.effect++
	if input.Name != "pay_invoice" {
		return nil, errors.New("unreviewed tool")
	}
	d.seen = input.Arguments.(map[string]any)
	if d.unknown {
		return nil, errors.New("lost provider response")
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "payment reported"}}, StructuredContent: map[string]any{"reported": true}}, nil
}

type sidecarFixture struct {
	setup              map[string]any
	mu                 sync.Mutex
	lostClaim          bool
	approval           bool
	deny               bool
	withhold           bool
	state              string
	claims, recoveries int
}

func (s *sidecarFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 40) {
		w.WriteHeader(401)
		return
	}
	write := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	switch r.URL.Path {
	case "/integration-operations":
		if s.setup == nil {
			w.WriteHeader(404)
			return
		}
		write(200, s.setup)
	case "/guard-requirements":
		write(200, map[string]any{"formatVersion": 1, "operationVersionId": operationID, "phase": r.URL.Query().Get("phase"), "bundleId": permitID, "sequence": 1, "coverage": "evaluated", "requirements": []any{}})
	case "/contextual-requirements":
		w.WriteHeader(404)
	case "/evaluate":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		outcome := "allow"
		if s.approval {
			outcome = "approval_required"
		}
		if s.deny || s.withhold && body["phase"] == "tool_result" {
			outcome = "deny"
		}
		write(200, map[string]any{"requestId": body["requestId"], "mode": "enforce", "enforced": false, "coverage": "evaluated", "outcome": outcome, "recordingStatus": "retained", "reportReplayed": false, "actionId": actionID, "approvalRequestId": approvalID, "bundleId": permitID, "sequence": 1})
	case "/execution-receipts":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		state := "succeeded"
		if body["event"] == "started" {
			state = "executing"
		}
		write(201, map[string]any{"receiptId": permitID, "state": state, "replayed": false})
	case "/approval-permits/status":
		write(200, map[string]any{"actionId": actionID, "approvalRequestId": approvalID, "state": s.state, "expiresAt": time.Now().Add(time.Hour)})
	case "/approval-permits/claim":
		s.claims++
		s.state = "consumed"
		if s.lostClaim {
			w.WriteHeader(503)
			return
		}
		write(201, map[string]any{"permitId": permitID, "actionId": actionID, "expiresAt": time.Now().Add(time.Minute)})
	case "/approval-permits/recover":
		s.recoveries++
		write(200, map[string]any{"permitId": permitID, "actionId": actionID, "expiresAt": time.Now().Add(time.Minute)})
	default:
		w.WriteHeader(404)
	}
}
func fixture(t *testing.T) (Config, *downstreamFixture, *sidecarFixture) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sidecar := &sidecarFixture{state: "pending"}
	httpServer := httptest.NewServer(sidecar)
	t.Cleanup(httpServer.Close)
	downstream := &downstreamFixture{tool: &mcp.Tool{Name: "pay_invoice", InputSchema: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"invoiceId": map[string]any{"type": "string"}, "amount": map[string]any{"type": "number"}, "idempotencyKey": map[string]any{"type": "string"}}, "required": []string{"invoiceId", "amount", "idempotencyKey"}}}}
	pin, _ := SchemaDigest(downstream.tool)
	journalKey := make([]byte, 32)
	_, _ = rand.Read(journalKey)
	config := Config{Runtime: authrail.Config{SidecarURL: httpServer.URL, AdapterToken: strings.Repeat("a", 40), OrganisationID: "44444444-4444-4444-8444-444444444444", DeploymentID: "55555555-5555-4555-8555-555555555555", ExecutorPrivateKey: key}, PrincipalID: "user-1", SessionID: "session-1", StateDirectory: filepath.Join(t.TempDir(), "journal"), JournalKey: journalKey, Downstream: downstream, Bindings: []Binding{{Name: "pay", Description: "Pay one reviewed invoice.", DownstreamName: "pay_invoice", OperationVersionID: operationID, SchemaSHA256: pin, IdempotencyField: "idempotencyKey", ResourceField: "invoiceId", Resources: map[string]string{"invoice-1": "invoice-1"}, ReviewFields: []string{"invoiceId", "amount"}}}}
	return config, downstream, sidecar
}
func request() invocation {
	return invocation{RequestKey: "payment-1", Arguments: map[string]any{"invoiceId": "invoice-1", "amount": 80}}
}

// Adapter permission, durable effect markers and result gating must agree across replay and restart.
func TestProtectedToolDecisionAndReplay(t *testing.T) {
	for _, kind := range []string{"allow", "deny", "unknown", "withheld"} {
		t.Run(kind, func(t *testing.T) {
			config, downstream, sidecar := fixture(t)
			sidecar.deny = kind == "deny"
			sidecar.withhold = kind == "withheld"
			downstream.unknown = kind == "unknown"
			connector, err := New(context.Background(), config)
			if err != nil {
				t.Fatal(err)
			}
			result, err := connector.invoke(context.Background(), "pay", request())
			if err != nil {
				t.Fatal(err)
			}
			expected := map[string]string{"allow": "completed", "deny": "blocked", "unknown": "outcome_unknown", "withheld": "result_withheld"}[kind]
			if result.Status != expected {
				t.Fatalf("got %s want %s", result.Status, expected)
			}
			if result.DeploymentID != config.Runtime.DeploymentID {
				t.Fatal("Action inspection is missing the authenticated deployment")
			}
			if kind == "allow" && result.Result == nil || kind != "allow" && result.Result != nil {
				t.Fatal("unchecked result exposure")
			}
			want := 1
			if kind == "deny" {
				want = 0
			}
			if downstream.effect != want {
				t.Fatal("unexpected effect count")
			}
			if want == 1 && downstream.seen["idempotencyKey"] != "ar-"+actionID {
				t.Fatal("downstream business idempotency was not fixed")
			}
			if err = connector.Close(); err != nil {
				t.Fatal(err)
			}
			connector, err = New(context.Background(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer connector.Close()
			again, err := connector.invoke(context.Background(), "pay", request())
			if err != nil || again.Result != nil || downstream.effect != want || again.DeploymentID != result.DeploymentID {
				t.Fatal("restart replay ran or exposed the effect")
			}
			changed := request()
			changed.Arguments["amount"] = 100
			if _, err = connector.invoke(context.Background(), "pay", changed); err == nil {
				t.Fatal("request key accepted changed intent")
			}
		})
	}
}
func TestApprovalSurvivesRestartAndWaitsWithoutClaim(t *testing.T) {
	config, downstream, sidecar := fixture(t)
	sidecar.approval = true
	connector, err := New(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := connector.invoke(context.Background(), "pay", request())
	if err != nil || waiting.Status != "awaiting_approval" || downstream.effect != 0 {
		t.Fatalf("waiting request failed: %+v %v", waiting, err)
	}
	files, _ := filepath.Glob(filepath.Join(config.StateDirectory, "*.state"))
	encrypted, _ := os.ReadFile(files[0])
	if strings.Contains(string(encrypted), "invoice-1") || strings.Contains(string(encrypted), "principalId") {
		t.Fatal("approval stored without encryption")
	}
	connector.Close()
	connector, err = New(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer connector.Close()
	pending, err := connector.resume(context.Background(), "pay", "payment-1")
	if err != nil || pending.Status != "awaiting_approval" || sidecar.claims != 0 {
		t.Fatal("pending approval was claimed")
	}
	sidecar.mu.Lock()
	sidecar.state = "approved"
	sidecar.mu.Unlock()
	done, err := connector.resume(context.Background(), "pay", "payment-1")
	if err != nil || done.Status != "completed" || downstream.effect != 1 || sidecar.claims != 1 {
		t.Fatalf("approved continuation failed: %+v %v", done, err)
	}
	_, err = connector.resume(context.Background(), "pay", "payment-1")
	if err != nil || downstream.effect != 1 {
		t.Fatal("approval replay ran twice")
	}
}
func TestUntrustedResourceAndSchemaDriftNeverExecute(t *testing.T) {
	config, downstream, _ := fixture(t)
	connector, err := New(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer connector.Close()
	input := request()
	input.Arguments["invoiceId"] = "other"
	if _, err = connector.invoke(context.Background(), "pay", input); err == nil || downstream.effect != 0 {
		t.Fatal("agent invented a trusted resource")
	}
	downstream.tool.InputSchema = map[string]any{"type": "object"}
	result, err := connector.invoke(context.Background(), "pay", request())
	if err != nil || result.Status != "outcome_unknown" || downstream.effect != 0 {
		t.Fatal("schema drift reached downstream")
	}
}

// Journal authentication and process exclusion prevent substitution and duplicate executors.
func TestPrivateStoreRejectsTamperingAndConcurrentOwners(t *testing.T) {
	root := filepath.Join(t.TempDir(), "journal")
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	store, err := openStore(root, key)
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	if other, err := openStore(root, key); err == nil {
		other.close()
		t.Fatal("two processes could share execution state")
	}
	id := stateKey("task")
	if err = store.save(id, &record{Binding: "reviewed", Intent: "intent", State: "started"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, id+".state")
	body, _ := os.ReadFile(path)
	body[len(body)-1] ^= 1
	_ = os.WriteFile(path, body, 0600)
	if _, err = store.load(id); err == nil {
		t.Fatal("tampered recovery state was accepted")
	}
}

// Losing the successful claim response must recover that permit, never claim or execute twice.
func TestLostApprovalClaimRecoversOnlyTheExistingPermission(t *testing.T) {
	config, downstream, sidecar := fixture(t)
	sidecar.approval = true
	sidecar.lostClaim = true
	connector, err := New(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = connector.invoke(context.Background(), "pay", request()); err != nil {
		t.Fatal(err)
	}
	sidecar.mu.Lock()
	sidecar.state = "approved"
	sidecar.mu.Unlock()
	result, err := connector.resume(context.Background(), "pay", "payment-1")
	if err != nil || result.Status != "claim_unknown" || downstream.effect != 0 {
		t.Fatalf("ambiguous claim executed: %+v %v", result, err)
	}
	connector.Close()
	connector, err = New(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer connector.Close()
	result, err = connector.resume(context.Background(), "pay", "payment-1")
	if err != nil || result.Status != "completed" || downstream.effect != 1 || sidecar.claims != 1 || sidecar.recoveries != 1 {
		t.Fatalf("existing claim recovery failed: %+v %v", result, err)
	}
}

// Real MCP client/server sessions must expose only fixed tools and return only checked output.
func TestMCPTransportProtectsTheDownstreamCall(t *testing.T) {
	config, downstream, _ := fixture(t)
	provider := mcp.NewServer(&mcp.Implementation{Name: "business", Version: "1"}, nil)
	provider.AddTool(downstream.tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
		return downstream.CallTool(ctx, &mcp.CallToolParams{Name: req.Params.Name, Arguments: args})
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return provider }, nil)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-business-token" || r.URL.Path != "/mcp" {
			t.Error("downstream private credential or path changed")
			w.WriteHeader(401)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("private-business-token"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	remote, err := ConnectDownstream(ctx, Settings{DownstreamURL: upstream.URL + "/mcp", DownstreamTokenFile: tokenFile, AllowLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	config.Downstream = remote
	connector, err := New(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer connector.Close()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := connector.Server().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tools, err := client.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 3 {
		t.Fatalf("unexpected tool discovery: %v %v", tools, err)
	}
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "pay", Arguments: request()})
	if err != nil || result.IsError || downstream.effect != 1 {
		t.Fatalf("protected MCP call failed: %+v %v", result, err)
	}
	body, _ := json.Marshal(result.StructuredContent)
	var checked reply
	if json.Unmarshal(body, &checked) != nil || checked.Status != "completed" || checked.Result == nil || !strings.Contains(checked.Result.Text, "payment reported") {
		t.Fatalf("checked result missing: %s", body)
	}
	_, _ = client.CallTool(ctx, &mcp.CallToolParams{Name: "pay_invoice", Arguments: request()})
	if downstream.effect != 1 {
		t.Fatal("unbound direct tool executed")
	}
}

func TestReconciliationUsesTrustedLookupWithoutRepeatingEffect(t *testing.T) {
	config, downstream, _ := fixture(t)
	downstream.unknown = true
	connector, err := New(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer connector.Close()
	if _, err = connector.invoke(context.Background(), "pay", request()); err != nil {
		t.Fatal(err)
	}
	err = connector.Reconcile(context.Background(), "pay", "payment-1", func(_ context.Context, recovery authrail.Recovery) (string, string, error) {
		if recovery.ProviderIdempotencyKey != "ar-"+actionID {
			t.Fatal("lookup lost original business idempotency")
		}
		return "provider-payment-1", "succeeded", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := connector.invoke(context.Background(), "pay", request())
	if err != nil || result.Status != "reconciled_succeeded" || result.Result != nil || downstream.effect != 1 {
		t.Fatalf("reconciliation repeated or exposed effect: %+v %v", result, err)
	}
}
