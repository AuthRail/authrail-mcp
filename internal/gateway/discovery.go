package gateway

import (
	"bytes"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const discoveryDeadline = 20 * time.Second
const maxDiscoveryTools = 128
const maxDiscoveryPages = 16

var discoveryToolName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
var discoveryBearer = regexp.MustCompile(`^Bearer [A-Za-z0-9._~+/-]+=*$`)

// Discovery reads an untrusted server's catalogue. It never grants tool access.
type Discovery struct {
	api      *API
	capacity chan struct{}
}

type DiscoveryInput struct {
	Endpoint      string `json:"endpoint"`
	Authorization string `json:"authorization"`
}
type DiscoveryServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Title   string `json:"title,omitempty"`
}
type DiscoveryTool struct {
	Name         string          `json:"name"`
	Title        string          `json:"title,omitempty"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
	Annotations  json.RawMessage `json:"annotations,omitempty"`
	Execution    json.RawMessage `json:"execution,omitempty"`
}
type DiscoverySnapshot struct {
	ServerInfo      DiscoveryServerInfo `json:"serverInfo"`
	ProtocolVersion string              `json:"protocolVersion"`
	Tools           []DiscoveryTool     `json:"tools"`
}

func NewDiscovery(api *API) *Discovery {
	return &Discovery{api: api, capacity: make(chan struct{}, 8)}
}

func (d *Discovery) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+d.api.token)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		var input DiscoveryInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 40<<10))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF ||
			len(input.Authorization) > 32768 || (input.Authorization != "" && !discoveryBearer.MatchString(input.Authorization)) {
			http.Error(w, "Invalid discovery request", http.StatusBadRequest)
			return
		}
		select {
		case d.capacity <- struct{}{}:
			defer func() { <-d.capacity }()
		default:
			http.Error(w, "Discovery busy", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), discoveryDeadline)
		defer cancel()
		client, transport, err := discoveryHTTPClient(ctx, input)
		if err != nil {
			http.Error(w, "Server address unavailable", http.StatusBadRequest)
			return
		}
		defer transport.CloseIdleConnections()
		snapshot, err := collectDiscovery(ctx, client, input.Endpoint)
		if err != nil {
			// Do not echo URLs, provider errors or credentials into the private response.
			http.Error(w, "Server tools unavailable", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snapshot)
	})
}

func discoveryHTTPClient(ctx context.Context, input DiscoveryInput) (*http.Client, *http.Transport, error) {
	if len(input.Endpoint) > 2048 || !validRoute(Route{Target: "discovery", Protocol: "mcp", Endpoint: input.Endpoint}) {
		return nil, nil, errBoundary
	}
	endpoint, _ := url.Parse(input.Endpoint)
	port := endpoint.Port()
	if port == "" {
		port = "443"
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", endpoint.Hostname())
	if err != nil || len(addresses) == 0 || len(addresses) > 32 {
		return nil, nil, errBoundary
	}
	for _, address := range addresses {
		if !publicAddress(address) {
			return nil, nil, errBoundary
		}
	}
	sort.Slice(addresses, func(i, j int) bool { return addresses[i].Compare(addresses[j]) < 0 })
	pinned := net.JoinHostPort(addresses[0].String(), port)
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, pinned)
		},
		TLSClientConfig:     &tls.Config{ServerName: endpoint.Hostname(), MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second,
		MaxResponseHeaderBytes: 16 << 10, MaxConnsPerHost: 2, ForceAttemptHTTP2: true,
	}
	return &http.Client{Timeout: discoveryDeadline, Transport: &discoveryHTTPTransport{base: transport, endpoint: input.Endpoint, authorization: input.Authorization},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, transport, nil
}

type discoveryHTTPTransport struct {
	base                    http.RoundTripper
	endpoint, authorization string
}

func (t *discoveryHTTPTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	// One validated origin/path and one DNS pin cover initialization, pages and cleanup.
	if request.URL.String() != t.endpoint {
		return nil, errBoundary
	}
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Del("Cookie")
	clone.Header.Del("Authorization")
	clone.Header.Del(ticketHeader)
	clone.Header.Del(dispatchHeader)
	if t.authorization != "" {
		clone.Header.Set("Authorization", t.authorization)
	}
	response, err := t.base.RoundTrip(clone)
	if err != nil {
		return nil, errBoundary
	}
	if response.ContentLength > maxBody {
		response.Body.Close()
		return nil, errOversized
	}
	response.Body = &discoveryBody{ReadCloser: response.Body, remaining: maxBody}
	return response, nil
}

type discoveryBody struct {
	io.ReadCloser
	remaining int64
}

func (b *discoveryBody) Read(buffer []byte) (int, error) {
	if int64(len(buffer)) > b.remaining+1 {
		buffer = buffer[:b.remaining+1]
	}
	n, err := b.ReadCloser.Read(buffer)
	b.remaining -= int64(n)
	if b.remaining < 0 {
		return 0, errOversized
	}
	return n, err
}

func collectDiscovery(ctx context.Context, client *http.Client, endpoint string) (*DiscoverySnapshot, error) {
	transport := &discoveryRecordingTransport{Transport: &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: client,
		MaxRetries: -1, DisableStandaloneSSE: true, MaxEventSize: maxBody}}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "authrail-tool-discovery", Version: "0.1.0"},
		&mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	session, err := mcpClient.Connect(ctx, transport, &mcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
	if err != nil {
		return nil, errBoundary
	}
	defer session.Close()
	initial := session.InitializeResult()
	if initial == nil || initial.ServerInfo == nil || initial.ServerInfo.Name == "" || initial.ServerInfo.Version == "" ||
		len(initial.ServerInfo.Name) > 200 || len(initial.ServerInfo.Version) > 100 || len(initial.ServerInfo.Title) > 200 {
		return nil, errBoundary
	}
	snapshot := &DiscoverySnapshot{ServerInfo: DiscoveryServerInfo{Name: initial.ServerInfo.Name, Version: initial.ServerInfo.Version, Title: initial.ServerInfo.Title},
		ProtocolVersion: initial.ProtocolVersion, Tools: []DiscoveryTool{}}
	cursor := ""
	seenCursors := map[string]bool{}
	seenNames := map[string]bool{}
	for page := 0; page < maxDiscoveryPages; page++ {
		if _, err = session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor}); err != nil {
			return nil, errBoundary
		}
		raw, err := transport.connection.result()
		if err != nil {
			return nil, err
		}
		var result struct {
			Tools      []DiscoveryTool `json:"tools"`
			NextCursor string          `json:"nextCursor"`
		}
		if json.Unmarshal(raw, &result) != nil || result.Tools == nil || len(result.NextCursor) > 4096 {
			return nil, errBoundary
		}
		for _, tool := range result.Tools {
			if !discoveryToolName.MatchString(tool.Name) || seenNames[tool.Name] || len(tool.Description) > 16000 || len(tool.Title) > 200 ||
				!discoveryObject(tool.InputSchema) || len(tool.OutputSchema) > 0 && !discoveryObject(tool.OutputSchema) ||
				len(tool.Annotations) > 0 && !discoveryObject(tool.Annotations) || len(tool.Execution) > 0 && !discoveryObject(tool.Execution) {
				return nil, errBoundary
			}
			seenNames[tool.Name] = true
			snapshot.Tools = append(snapshot.Tools, tool)
			if len(snapshot.Tools) > maxDiscoveryTools {
				return nil, errOversized
			}
		}
		encoded, err := json.Marshal(snapshot)
		if err != nil || len(encoded) > maxBody {
			return nil, errOversized
		}
		if result.NextCursor == "" {
			sort.Slice(snapshot.Tools, func(i, j int) bool { return snapshot.Tools[i].Name < snapshot.Tools[j].Name })
			return snapshot, nil
		}
		if seenCursors[result.NextCursor] {
			return nil, errBoundary
		}
		seenCursors[result.NextCursor] = true
		cursor = result.NextCursor
	}
	return nil, errOversized
}
func discoveryObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) >= 2 && trimmed[0] == '{' && json.Valid(trimmed)
}

// Preserve schema numeric literals instead of SDK decoding/re-encoding via float64.
type discoveryRecordingTransport struct {
	mcp.Transport
	connection *discoveryRecordingConnection
}

func (t *discoveryRecordingTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	connection, err := t.Transport.Connect(ctx)
	if err != nil {
		return nil, err
	}
	t.connection = &discoveryRecordingConnection{Connection: connection}
	return t.connection, nil
}

type discoveryRecordingConnection struct {
	mcp.Connection
	mu      sync.Mutex
	id      jsonrpc.ID
	pending bool
	raw     json.RawMessage
}

func (c *discoveryRecordingConnection) Write(ctx context.Context, message jsonrpc.Message) error {
	if request, ok := message.(*jsonrpc.Request); ok {
		switch request.Method {
		case "initialize", "notifications/initialized", "notifications/cancelled":
		case "tools/list":
			c.mu.Lock()
			c.id, c.pending, c.raw = request.ID, true, nil
			c.mu.Unlock()
		default:
			return errBoundary
		}
	}
	return c.Connection.Write(ctx, message)
}
func (c *discoveryRecordingConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	message, err := c.Connection.Read(ctx)
	if err != nil {
		return nil, err
	}
	if response, ok := message.(*jsonrpc.Response); ok {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.pending && response.ID == c.id && response.Error == nil {
			if len(response.Result) > maxBody {
				return nil, errOversized
			}
			c.raw = bytes.Clone(response.Result)
			c.pending = false
		}
	}
	return message, nil
}
func (c *discoveryRecordingConnection) result() (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.raw) == 0 {
		return nil, errBoundary
	}
	return bytes.Clone(c.raw), nil
}

// Key order is irrelevant; json.Number keeps schema constants out of float64.
// Equivalent decimal spellings survive JSONB round trips without using float64.
func sameDiscoveryTool(expected, current DiscoveryTool) bool {
	canonical := func(tool DiscoveryTool) ([]byte, error) {
		encoded, err := json.Marshal(tool)
		if err != nil {
			return nil, err
		}
		var value interface{}
		decoder := json.NewDecoder(bytes.NewReader(encoded))
		decoder.UseNumber()
		if err = decoder.Decode(&value); err != nil {
			return nil, err
		}
		return json.Marshal(normalizeDiscoveryNumbers(value))
	}
	left, err := canonical(expected)
	if err != nil {
		return false
	}
	right, err := canonical(current)
	return err == nil && bytes.Equal(left, right)
}

func normalizeDiscoveryNumbers(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			typed[key] = normalizeDiscoveryNumbers(child)
		}
	case []interface{}:
		for index, child := range typed {
			typed[index] = normalizeDiscoveryNumbers(child)
		}
	case json.Number:
		text := strings.ToLower(string(typed))
		negative := strings.HasPrefix(text, "-")
		text = strings.TrimPrefix(text, "-")
		coefficient, exponent, _ := strings.Cut(text, "e")
		power := new(big.Int)
		if exponent != "" {
			power.SetString(exponent, 10)
		}
		if dot := strings.IndexByte(coefficient, '.'); dot >= 0 {
			power.Sub(power, big.NewInt(int64(len(coefficient)-dot-1)))
			coefficient = strings.ReplaceAll(coefficient, ".", "")
		}
		coefficient = strings.TrimLeft(coefficient, "0")
		if coefficient == "" {
			return json.Number("0")
		}
		trimmed := strings.TrimRight(coefficient, "0")
		power.Add(power, big.NewInt(int64(len(coefficient)-len(trimmed))))
		if negative {
			trimmed = "-" + trimmed
		}
		return json.Number(trimmed + "e" + power.String())
	}
	return value
}

func (d *Dispatcher) checkReviewedTool(ctx context.Context, ticket string, resolution Resolution, authorization *string) error {
	if resolution.ReviewedTool == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, discoveryDeadline)
	defer cancel()
	input := DiscoveryInput{Endpoint: resolution.Endpoint}
	if authorization != nil {
		input.Authorization = *authorization
	}
	var snapshot *DiscoverySnapshot
	var err error
	if d.toolDiscovery != nil {
		snapshot, err = d.toolDiscovery(ctx, input)
	} else {
		client, transport, clientErr := discoveryHTTPClient(ctx, input)
		if clientErr != nil {
			return errBoundary
		}
		defer transport.CloseIdleConnections()
		snapshot, err = collectDiscovery(ctx, client, resolution.Endpoint)
	}
	if err != nil || snapshot == nil {
		return errBoundary
	}
	for _, tool := range snapshot.Tools {
		if tool.Name == resolution.ReviewedTool.Name && sameDiscoveryTool(*resolution.ReviewedTool, tool) {
			// Task-required tools cannot be sent through bounded tools/call.
			var execution struct {
				TaskSupport string `json:"taskSupport"`
			}
			if len(tool.Execution) > 0 && (json.Unmarshal(tool.Execution, &execution) != nil || execution.TaskSupport == "required") {
				break
			}
			return nil
		}
	}
	if err := d.api.schemaChange(ctx, ticket, resolution.Target); err != nil {
		return err
	}
	return errBoundary
}
