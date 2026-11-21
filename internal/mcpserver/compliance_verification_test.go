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

func TestComplianceVerificationNamedBoundaries(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000001"
	var calls atomic.Int32
	var wrongIdentity atomic.Bool
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/mcp/introspect" {
			_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user-1", OrganisationID: "org-1", ClientID: "client-1", SessionID: "session-1", Scopes: []string{Scope, ComplianceVerificationManageScope}, ExpiresAt: time.Now().Add(time.Minute)})
			return
		}
		if r.URL.Path == "/auth/mcp/delegations/exchange" {
			var body struct{ Token, Tool string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": body.Tool, "expiresAt": time.Now().Add(time.Minute)})
			return
		}
		tool := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if r.Header.Get("X-Organisation-Id") != "org-1" || r.Method != http.MethodPost {
			t.Error("lost delegated audience or method")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch tool {
		case "authrail_save_test_definition":
			if r.URL.Path != "/organisation/mcp/verification/test-definitions" || body["controlVersionId"] != id || body["revision"] != float64(7) || body["idempotencyKey"] != id {
				t.Error("lost exact owner revision or command identity")
			}
		case "authrail_update_remediation_task":
			if r.URL.Path != "/organisation/mcp/verification/tasks/"+id {
				t.Error("wrong Task route")
			}
			if value, ok := body["assigneeStaffId"]; !ok || value != nil {
				t.Error("explicit clear-assignee did not reach API as null")
			}
			if value, ok := body["dueAt"]; !ok || value != nil {
				t.Error("explicit clear-due-date did not reach API as null")
			}
			if _, ok := body["ownerStaffId"]; ok {
				t.Error("omitted unchanged owner was supplied")
			}
			if _, ok := body["clearAssignee"]; ok {
				t.Error("tool-only clear flag reached API")
			}
		case "authrail_review_evidence_record":
			if r.URL.Path != "/organisation/mcp/verification/evidence-records/review-handoff" || body["recordId"] != id || body["definitionId"] != id {
				t.Error("lost exact private review target")
			}
		default:
			t.Errorf("unexpected tool: %s", tool)
		}
		calls.Add(1)
		user := "user-1"
		if wrongIdentity.Load() {
			user = "other-user"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": "org-1", "userId": user, "data": map[string]any{"navigationOnly": true}})
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
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "verification-boundaries", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: resource, HTTPClient: &http.Client{Transport: bearerTransport{strings.Repeat("u", 32)}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	draft := map[string]any{"controlId": id, "controlVersionId": id, "id": id, "revision": 7, "idempotencyKey": id, "title": "Exact draft", "procedure": "Check selected retained records.", "cadenceDays": 30, "freshnessDays": 60}
	review := map[string]any{"controlId": id, "controlVersionId": id, "definitionId": id, "recordId": id}
	for name, args := range map[string]map[string]any{"authrail_save_test_definition": draft, "authrail_review_evidence_record": review, "authrail_update_remediation_task": {"taskId": id, "idempotencyKey": id, "revision": 7, "state": "in_progress", "reason": "Retain current work.", "clearAssignee": true, "clearDueDate": true}} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || result.IsError {
			t.Fatalf("named workflow failed: %s: %v %v", name, result, err)
		}
	}
	before := calls.Load()
	invalid := map[string]any{"controlId": id, "controlVersionId": id, "definitionId": id, "recordId": "not-an-exact-record"}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_review_evidence_record", Arguments: invalid})
	if err == nil && !result.IsError {
		t.Fatal("malformed exact record accepted")
	}
	if calls.Load() != before {
		t.Fatal("malformed target reached API")
	}
	wrongIdentity.Store(true)
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "authrail_save_test_definition", Arguments: draft})
	if err == nil && !result.IsError {
		t.Fatal("mismatched response identity released")
	}
}
