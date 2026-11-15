package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiscoveryPreservesSchemasAndOnlyListsTools(t *testing.T) {
	var lists atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer account-fixture" || r.Header.Get("Cookie") != "" || r.Header.Get(ticketHeader) != "" {
			t.Error("discovery credential boundary changed")
			http.Error(w, "denied", http.StatusForbidden)
			return
		}
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("invalid protocol request")
			return
		}
		if request.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var result string
		switch request.Method {
		case "initialize":
			result = `{"protocolVersion":"2025-11-25","serverInfo":{"name":"fixture","version":"1"},"capabilities":{"tools":{}}}`
		case "tools/list":
			if lists.Add(1) == 1 {
				result = `{"tools":[{"name":"z_create","description":"Create ticket","inputSchema":{"type":"object","properties":{"number":{"const":9007199254740993}}},"annotations":{"readOnlyHint":false}}],"nextCursor":"second"}`
			} else {
				if !bytes.Contains(request.Params, []byte(`"cursor":"second"`)) {
					t.Error("pagination cursor lost")
				}
				result = `{"tools":[{"name":"a_find","inputSchema":{"type":"object"},"outputSchema":{"type":"object"}}]}`
			}
		default:
			t.Errorf("discovery sent non-catalogue method %s", request.Method)
			http.Error(w, "denied", http.StatusForbidden)
			return
		}
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":`+string(request.ID)+`,"result":`+result+`}`)
	}))
	defer server.Close()
	client := &http.Client{Transport: &discoveryHTTPTransport{base: http.DefaultTransport, endpoint: server.URL, authorization: "Bearer account-fixture"}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	snapshot, err := collectDiscovery(ctx, client, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if lists.Load() != 2 || len(snapshot.Tools) != 2 || snapshot.Tools[0].Name != "a_find" {
		t.Fatalf("wrong catalogue: %+v", snapshot)
	}
	if !bytes.Contains(snapshot.Tools[1].InputSchema, []byte("9007199254740993")) {
		t.Fatal("schema number rounded")
	}
}

func TestDiscoveryRejectsLocalDestinationsAndNonServiceRequests(t *testing.T) {
	for _, endpoint := range []string{"http://example.com/mcp", "https://127.0.0.1/mcp", "https://[::1]/mcp", "https://169.254.169.254/mcp", "https://user:pass@example.com/mcp", "https://example.com/mcp?token=secret", "https://example.com/mcp#fragment"} {
		client, transport, err := discoveryHTTPClient(context.Background(), DiscoveryInput{Endpoint: endpoint})
		if err == nil {
			transport.CloseIdleConnections()
			t.Fatalf("accepted forbidden destination %s (%v)", endpoint, client != nil)
		}
	}
	discovery := NewDiscovery(&API{token: "private-service-fixture-token"})
	request := httptest.NewRequest(http.MethodPost, "/discover", strings.NewReader(`{"endpoint":"https://example.com/mcp","authorization":"Bearer account-fixture"}`))
	response := httptest.NewRecorder()
	discovery.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatal("discovery exposed without service authorization")
	}
}

func TestDiscoveryTransportDoesNotForwardRedirectCredentials(t *testing.T) {
	var reached atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Add(1) }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	client := &http.Client{Transport: &discoveryHTTPTransport{base: http.DefaultTransport, endpoint: source.URL, authorization: "Bearer fixture"},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Post(source.URL, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if reached.Load() != 0 || response.StatusCode != http.StatusTemporaryRedirect {
		t.Fatal("redirect escaped fixed discovery destination")
	}
	request, _ := http.NewRequest(http.MethodPost, destination.URL, nil)
	if _, err := client.Do(request); err == nil {
		t.Fatal("transport allowed a different endpoint")
	}
}

func TestReviewedToolComparisonDetectsNumericAndPermissionChanges(t *testing.T) {
	if !sameDiscoveryTool(DiscoveryTool{Name: "decimal", InputSchema: json.RawMessage(`{"minimum":1,"multipleOf":0.01}`)},
		DiscoveryTool{Name: "decimal", InputSchema: json.RawMessage(`{"multipleOf":1e-2,"minimum":1.0}`)}) {
		t.Fatal("equivalent decimal spellings invalidated a JSONB-backed review")
	}
	expected := DiscoveryTool{Name: "create_ticket", InputSchema: json.RawMessage(`{"type":"object","properties":{"number":{"const":9007199254740993}}}`),
		Annotations: json.RawMessage(`{"readOnlyHint":false,"destructiveHint":false}`)}
	current := DiscoveryTool{Name: "create_ticket", InputSchema: json.RawMessage(`{"properties":{"number":{"const":9007199254740993}},"type":"object"}`),
		Annotations: json.RawMessage(`{"destructiveHint":false,"readOnlyHint":false}`)}
	if !sameDiscoveryTool(expected, current) {
		t.Fatal("key order unexpectedly invalidated review")
	}
	current.InputSchema = json.RawMessage(`{"properties":{"number":{"const":9007199254740992}},"type":"object"}`)
	if sameDiscoveryTool(expected, current) {
		t.Fatal("large schema constants collapsed into one reviewed tool")
	}
	current.InputSchema = expected.InputSchema
	current.Annotations = json.RawMessage(`{"destructiveHint":true,"readOnlyHint":false}`)
	if sameDiscoveryTool(expected, current) {
		t.Fatal("changed tool declaration retained previous review")
	}
}
