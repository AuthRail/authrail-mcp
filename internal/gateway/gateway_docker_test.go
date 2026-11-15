package gateway

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AuthRail/authrail-mcp/internal/gateway/wire"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"
)

// One explicit protocol check against the pinned binary. All accounts, keys and
// effects belong to this isolated fixture; the normal product has no seeded route.
func TestPinnedGatewayDispatch(t *testing.T) {
	if os.Getenv("AUTHRAIL_GATEWAY_DOCKER_VERIFICATION") != "1" {
		t.Skip("explicit local Docker verification only")
	}
	const serviceToken = "isolated-gateway-fixture-service-credential"
	const providerToken = "isolated-provider-token"
	var effects atomic.Int32
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "create_issue", InputSchema: map[string]interface{}{"type": "object"}},
		func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !bytes.Contains(request.Params.Arguments, []byte("9007199254740993")) {
				t.Error("number changed before provider call")
			}
			effects.Add(1)
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "created"}}}, nil
		})
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, DisableLocalhostProtection: true})
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+providerToken || r.Header.Get(ticketHeader) != "" || r.Header.Get(dispatchHeader) != "" {
			t.Errorf("upstream isolation: authorized=%v ticketPresent=%v transportCredentialPresent=%v", r.Header.Get("Authorization") == "Bearer "+providerToken, r.Header.Get(ticketHeader) != "", r.Header.Get(dispatchHeader) != "")
			http.Error(w, "denied", 403)
			return
		}
		if r.URL.Path == "/a2a" {
			effects.Add(1)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"jsonrpc":"2.0","id":"authrail","result":{"id":"fixture-task"}}`)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	certificate, root := fixtureCertificate(t)
	upstream.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	upstream.StartTLS()
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "root.pem"), root, 0644); err != nil {
		t.Fatal(err)
	}
	routes := []Route{{Target: "ar-11111111111111111111111111111111", Protocol: "mcp", Endpoint: "https://example.com:" + upstreamURL.Port() + "/mcp"},
		{Target: "ar-22222222222222222222222222222222", Protocol: "a2a", Endpoint: "https://example.com:" + upstreamURL.Port() + "/a2a"}}
	params := []string{`{"name":"create_issue","arguments":{"amount":9007199254740993}}`,
		`{"message":{"messageId":"fixture","role":"user","parts":[{"kind":"text","text":"hello"}]}}`,
		`{"name":"stripe.refund_charge","arguments":{"amount":9007199254740993}}`}
	var calls [3]atomic.Int32
	var responses [3]atomic.Int32
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/gateway/provider" {
			encoded, _ := io.ReadAll(r.Body)
			var envelope struct {
				Params string `json:"params"`
			}
			decodeErr := json.Unmarshal(encoded, &envelope)
			if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("t", 40) || r.Header.Get(dispatchHeader) != "" ||
				r.Header.Get(ticketHeader) != "" || r.Header.Get("Cookie") != "" || decodeErr != nil || envelope.Params != params[2] {
				t.Error("native API did not receive the exact attempt credential/body isolation")
				http.Error(w, "denied", http.StatusForbidden)
				return
			}
			effects.Add(1)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"state":"accepted","providerReference":"re_fixture"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+serviceToken {
			http.Error(w, "denied", 403)
			return
		}
		var input struct{ Ticket, Target, Protocol, Method, Params, AttemptToken, Result string }
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid", 400)
			return
		}
		index := 0
		if input.Ticket == strings.Repeat("b", 40) || input.Target == routes[1].Target {
			index = 1
		} else if input.Ticket == strings.Repeat("c", 40) || len(routes) > 2 && input.Target == routes[2].Target {
			index = 2
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/internal/gateway/resolve":
			method := "tools/call"
			if index == 1 {
				method = "message/send"
			} else if index == 2 {
				method = http.MethodPost
			}
			json.NewEncoder(w).Encode(Resolution{Route: routes[index], Method: method, Params: params[index], DeadlineAt: time.Now().Add(50 * time.Second)})
		case "/internal/gateway/request":
			if input.Method == "tools/call" || input.Method == "message/send" || input.Method == http.MethodPost {
				if calls[index].Add(1) != 1 {
					http.Error(w, "already dispatched", 409)
					return
				}
			}
			authorization := "Bearer " + providerToken
			if index == 2 {
				authorization = "Bearer " + strings.Repeat("t", 40)
			}
			json.NewEncoder(w).Encode(RequestPermission{Allowed: true, AttemptToken: strings.Repeat("t", 40), Authorization: &authorization, DeadlineAt: time.Now().Add(50 * time.Second)})
		case "/internal/gateway/response":
			responses[index].Add(1)
			json.NewEncoder(w).Encode(retained{true})
		case "/internal/gateway/failure":
			json.NewEncoder(w).Encode(retained{true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()
	routes = append(routes, Route{Target: "ar-33333333333333333333333333333333", Protocol: "http", Endpoint: apiServer.URL + "/internal/gateway/provider"})
	api, err := NewAPI(apiServer.URL, serviceToken, true)
	if err != nil {
		t.Fatal(err)
	}
	grpcListener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpc.NewServer()
	wire.RegisterExtMcpServer(grpcServer, &Hooks{API: api})
	go grpcServer.Serve(grpcListener)
	defer grpcServer.Stop()
	fixtureConfiguration := &Configuration{key: strings.Repeat("1", 64)}
	encoded, err := configurationJSON(routes, "host.docker.internal:"+strings.Split(grpcListener.Addr().String(), "]:")[1], fixtureConfiguration.key, fixtureConfiguration.transportToken(), map[string]string{routes[0].Target: "host.docker.internal", routes[1].Target: "host.docker.internal", routes[2].Target: "host.docker.internal"})
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]interface{}
	if json.Unmarshal(encoded, &config) != nil {
		t.Fatal("configuration JSON")
	}
	delete(config["frontendPolicies"].(map[string]interface{}), "tracing")
	config["frontendPolicies"].(map[string]interface{})["accessLog"].(map[string]interface{})["filter"] = "true"
	configuredRoutes := config["binds"].([]interface{})[0].(map[string]interface{})["listeners"].([]interface{})[0].(map[string]interface{})["routes"].([]interface{})
	for _, route := range configuredRoutes {
		backend := route.(map[string]interface{})["backends"].([]interface{})[0].(map[string]interface{})
		policies := backend["policies"]
		if mcpBackend, ok := backend["mcp"]; ok {
			policies = mcpBackend.(map[string]interface{})["targets"].([]interface{})[0].(map[string]interface{})["policies"]
		}
		if tls, ok := policies.(map[string]interface{})["backendTLS"]; ok {
			tls.(map[string]interface{})["root"] = "/verification/root.pem"
		}
	}
	encoded, _ = json.Marshal(config)
	if err = os.WriteFile(filepath.Join(dir, "config.json"), encoded, 0644); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("docker", "run", "-d", "--add-host", "host.docker.internal:host-gateway", "-p", "127.0.0.1::3018", "-p", "127.0.0.1::15021", "-v", dir+":/verification:ro", "authrail-agentgateway:1.7.0-slice6", "-f", "/verification/config.json").CombinedOutput()
	if err != nil {
		t.Fatalf("isolated gateway startup: %s", output)
	}
	id := strings.TrimSpace(string(output))
	defer exec.Command("docker", "rm", "-f", id).Run()
	portOutput, err := exec.Command("docker", "port", id, "3018/tcp").Output()
	if err != nil {
		logs, _ := exec.Command("docker", "logs", id).CombinedOutput()
		t.Fatalf("gateway port unavailable: %v: %s", err, logs)
	}
	origin := "http://" + strings.TrimSpace(string(portOutput))
	readyPort, _ := exec.Command("docker", "port", id, "15021/tcp").Output()
	readyURL := "http://" + strings.TrimSpace(string(readyPort)) + "/healthz/ready"
	client := http.Client{Timeout: time.Second}
	ready := false
	for until := time.Now().Add(12 * time.Second); time.Now().Before(until); {
		response, e := client.Get(readyURL)
		if e == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	if !ready {
		logs, _ := exec.Command("docker", "logs", id).CombinedOutput()
		t.Fatalf("pinned configuration did not start: %s", logs)
	}
	// A service credential is not the gateway's private transport authority.
	bypass, _ := http.NewRequest(http.MethodPost, origin+"/a2a/"+routes[1].Target, strings.NewReader(`{"jsonrpc":"2.0","id":"authrail","method":"message/send","params":`+params[1]+`}`))
	bypass.Header.Set("Content-Type", "application/json")
	bypass.Header.Set("Authorization", "Bearer "+providerToken)
	bypass.Header.Set(dispatchHeader, (&Configuration{key: serviceToken}).transportToken())
	bypassResponse, err := http.DefaultClient.Do(bypass)
	if err != nil {
		t.Fatal(err)
	}
	bypassResponse.Body.Close()
	if bypassResponse.StatusCode != 403 || effects.Load() != 0 {
		t.Fatal("private gateway allowed dispatcher bypass")
	}
	nativeBypass, _ := http.NewRequest(http.MethodPost, origin+"/http/"+routes[2].Target, strings.NewReader(params[2]))
	nativeBypass.Header.Set("Content-Type", "application/json")
	nativeBypass.Header.Set("Authorization", "Bearer "+strings.Repeat("t", 40))
	nativeBypass.Header.Set(dispatchHeader, (&Configuration{key: serviceToken}).transportToken())
	nativeBypassResponse, err := http.DefaultClient.Do(nativeBypass)
	if err != nil {
		t.Fatal(err)
	}
	nativeBypassResponse.Body.Close()
	if nativeBypassResponse.StatusCode != 403 || effects.Load() != 0 {
		t.Fatal("native HTTP allowed dispatcher bypass")
	}
	configuration := &Configuration{key: fixtureConfiguration.key, routes: map[string]Route{routes[0].Target: routes[0], routes[1].Target: routes[1], routes[2].Target: routes[2]}, revision: "fixture", updated: time.Now()}
	dispatcher, err := NewDispatcher(api, configuration, origin, true)
	if err != nil {
		t.Fatal(err)
	}
	for index := range routes {
		ticket := strings.Repeat("a", 40)
		if index == 1 {
			ticket = strings.Repeat("b", 40)
		} else if index == 2 {
			ticket = strings.Repeat("c", 40)
		}
		req := httptest.NewRequest(http.MethodPost, "http://private/dispatch", strings.NewReader(`{"ticket":"`+ticket+`"}`))
		req.Header.Set("Authorization", "Bearer "+serviceToken)
		recorder := httptest.NewRecorder()
		dispatcher.Handler().ServeHTTP(recorder, req)
		if recorder.Code != 200 {
			logs, _ := exec.Command("docker", "logs", id).CombinedOutput()
			t.Fatalf("%s private dispatch %d: %s\n%s", routes[index].Protocol, recorder.Code, recorder.Body.String(), logs)
		}
		if calls[index].Load() != 1 || responses[index].Load() != 1 {
			t.Fatalf("%s missing exact request/result hook", routes[index].Protocol)
		}
	}
	if effects.Load() != 3 {
		t.Fatalf("expected three isolated effects, got %d", effects.Load())
	}
}

func TestPublicDestinationBoundary(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "100.64.0.1", "169.254.169.254", "::1", "::ffff:127.0.0.1", "64:ff9b::a00:1", "2002:a00:1::"} {
		if publicAddress(netip.MustParseAddr(raw)) {
			t.Errorf("private or translated address accepted: %s", raw)
		}
	}
	if !publicAddress(netip.MustParseAddr("8.8.8.8")) {
		t.Fatal("public address rejected")
	}
}

func fixtureCertificate(t *testing.T) (tls.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Isolated fixture CA"},
		NotBefore: now.Add(-48 * time.Hour), NotAfter: now.Add(48 * time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "example.com"}, DNSNames: []string{"example.com"},
		NotBefore: root.NotBefore, NotAfter: root.NotAfter, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, root, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatal(err)
	}
	return certificate, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})
}
