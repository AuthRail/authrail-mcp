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

const fileTestID = "10000000-0000-4000-8000-000000000001"
const fileTestOrg = "10000000-0000-4000-8000-000000000002"

func fileTestStatus() map[string]any {
	return map[string]any{"fileId": fileTestID, "routeId": fileTestID, "status": "awaiting_selection", "audience": "actor", "reference": nil, "argumentsSha256": strings.Repeat("a", 64), "expiresAt": nil, "boundToAction": false}
}
func TestBusinessFilesRefusePrivateCapabilitiesAndCrossTenantHandoff(t *testing.T) {
	for _, change := range []string{"upload", "object-key", "different-original", "private-reference", "handoff-tenant", "handoff-signature"} {
		t.Run(change, func(t *testing.T) {
			data := fileTestStatus()
			prepare := false
			expected := ""
			switch change {
			case "upload":
				data["upload"] = map[string]any{"url": "https://storage.example/private?signature=secret"}
			case "object-key":
				data["objectKey"] = "private/owner/source"
			case "different-original":
				expected = fileTestOrg
			case "private-reference":
				data["reference"] = map[string]any{"id": fileTestID, "filename": "file.pdf", "contentType": "application/pdf", "contentSha256": strings.Repeat("a", 64), "sizeBytes": float64(3), "url": "https://private.example/file"}
			case "handoff-tenant":
				prepare = true
				data["handoff"] = map[string]any{"url": "https://app.authrail.io/runtime/files/" + fileTestID + "?organisationId=" + fileTestID, "requiresHuman": true}
			case "handoff-signature":
				prepare = true
				data["handoff"] = map[string]any{"url": "https://app.authrail.io/runtime/files/" + fileTestID + "?organisationId=" + fileTestOrg + "&signature=secret", "requiresHuman": true}
			}
			if _, err := safeBusinessFile(data, prepare, expected, fileTestOrg); err == nil {
				t.Fatal("private or substituted file result accepted")
			}
		})
	}
}
func TestBusinessFilePreparationKeepsExactNumbersAndRejectsUnknownInputs(t *testing.T) {
	var input businessFilePrepareInput
	if err := json.Unmarshal([]byte(`{"arguments":{"value":9007199254740993},"audience":"actor"}`), &input); err != nil {
		t.Fatal(err)
	}
	if value, ok := input.Arguments["value"].(json.Number); !ok || value.String() != "9007199254740993" {
		t.Fatal("rounded reviewed JSON")
	}
	if json.Unmarshal([]byte(`{"arguments":{},"url":"https://unreviewed.example/upload"}`), &input) == nil {
		t.Fatal("accepted arbitrary transfer URL")
	}
}
func TestBusinessFileToolsUseNamedExecutionDelegationAndSafeOriginalStatus(t *testing.T) {
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/mcp/introspect":
			_ = json.NewEncoder(w).Encode(Identity{Active: true, UserID: "user", SessionID: "session", OrganisationID: fileTestOrg, ClientID: "client", Scopes: []string{Scope, BusinessExecuteScope}, ExpiresAt: time.Now().Add(time.Minute)})
		case "/auth/mcp/delegations/exchange":
			var input struct{ Token, Tool string }
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				t.Error("invalid delegation")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "delegation-" + input.Tool, "expiresAt": time.Now().Add(time.Minute)})
		default:
			calls.Add(1)
			name := "authrail_get_business_file"
			data := fileTestStatus()
			if r.URL.Path == "/organisation/mcp/business-files" {
				name = "authrail_prepare_business_file"
				var input businessFilePrepareInput
				if json.NewDecoder(r.Body).Decode(&input) != nil || input.Audience != "actor" || input.Arguments["issueKey"] != "ART-1" {
					t.Error("changed file preparation")
				}
				data["handoff"] = map[string]any{"url": "https://app.authrail.io/runtime/files/" + fileTestID + "?organisationId=" + fileTestOrg, "requiresHuman": true}
			} else if r.URL.Path == "/organisation/mcp/business-files/"+fileTestID+"/revoke" {
				name = "authrail_revoke_business_file"
				data["status"] = "revoked"
			} else if r.URL.Path != "/organisation/mcp/business-files/"+fileTestID {
				t.Error("unexpected file route")
			}
			if r.Header.Get("Authorization") != "Bearer delegation-"+name || r.Header.Get("X-Organisation-Id") != fileTestOrg {
				t.Error("lost named tenant delegation")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": fileTestOrg, "userId": "user", "data": data})
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "file-fixture", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: resource, HTTPClient: &http.Client{Transport: bearerTransport{strings.Repeat("u", 32)}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{"authrail_prepare_business_file", map[string]any{"routeId": fileTestID, "bindingSha256": strings.Repeat("a", 64), "idempotencyKey": fileTestID, "audience": "actor", "arguments": map[string]any{"issueKey": "ART-1"}}},
		{"authrail_get_business_file", map[string]any{"fileId": fileTestID}},
		{"authrail_revoke_business_file", map[string]any{"fileId": fileTestID}},
	} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: call.name, Arguments: call.args})
		if err != nil || result.IsError {
			t.Fatalf("%s: %v %v", call.name, result, err)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("unexpected file API calls: %d", calls.Load())
	}
}
