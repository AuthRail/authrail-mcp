package gateway

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"hash/fnv"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type Dispatcher struct {
	api            *API
	configuration  *Configuration
	origin         string
	client         *http.Client
	capacity       chan struct{}
	targetCapacity [64]chan struct{}
}

func NewDispatcher(api *API, configuration *Configuration, origin string, allowPrivateHTTP bool) (*Dispatcher, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") || (u.Scheme != "https" && !(allowPrivateHTTP && u.Scheme == "http")) {
		return nil, errBoundary
	}
	d := &Dispatcher{api: api, configuration: configuration, origin: origin,
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, capacity: make(chan struct{}, 32)}
	for index := range d.targetCapacity {
		d.targetCapacity[index] = make(chan struct{}, 4)
	}
	return d, nil
}

func (d *Dispatcher) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, _ *http.Request) {
		if !d.configuration.Ready() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /dispatch", d.dispatch)
	return mux
}

func (d *Dispatcher) dispatch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+d.api.token)) != 1 {
		http.Error(w, "Unauthorized", 401)
		return
	}
	select {
	case d.capacity <- struct{}{}:
		defer func() { <-d.capacity }()
	default:
		http.Error(w, "Gateway busy", 503)
		return
	}
	var input struct {
		Ticket string `json:"ticket"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 20<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF || !validTicket(input.Ticket) {
		http.Error(w, "Invalid dispatch", 400)
		return
	}
	resolution, err := d.api.resolve(r.Context(), input.Ticket)
	if err != nil || !d.configuration.Matches(resolution.Route) {
		http.Error(w, "Dispatch unavailable", 409)
		return
	}
	trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("authrail.action.id", resolution.ActionID), attribute.String("authrail.attempt.id", resolution.AttemptID))
	// Fixed shards bound memory and keep one route from occupying every worker.
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(resolution.Target))
	capacity := d.targetCapacity[hash.Sum32()%64]
	select {
	case capacity <- struct{}{}:
		defer func() { <-capacity }()
	default:
		http.Error(w, "Account busy", 503)
		return
	}
	ctx, cancel := context.WithDeadline(r.Context(), resolution.DeadlineAt)
	defer cancel()
	var result json.RawMessage
	if resolution.Protocol == "mcp" {
		result, err = d.callMCP(ctx, input.Ticket, resolution)
	} else {
		result, err = d.callA2A(ctx, input.Ticket, resolution)
	}
	if err != nil {
		kind := "transport"
		if errors.Is(err, errOversized) {
			kind = "oversized"
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			kind = "timeout"
		} else if errors.Is(err, errRPC) {
			kind = "rpc_error"
		}
		if !d.api.failure(ctx, input.Ticket, resolution.Target, kind) {
			slog.WarnContext(ctx, "Gateway outcome recording unavailable; original attempt requires recovery",
				"actionId", resolution.ActionID, "attemptId", resolution.AttemptID,
				"traceId", trace.SpanFromContext(ctx).SpanContext().TraceID().String())
		}
		http.Error(w, "Result remains pending recovery", 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}{"result_pending_check", result})
}

var errRPC = errors.New("upstream RPC did not complete")

func (d *Dispatcher) callA2A(ctx context.Context, ticket string, resolution Resolution) (json.RawMessage, error) {
	// Streaming/task subscriptions need their own durable lifecycle, not a read-all fallback.
	if resolution.Method != "SendMessage" && resolution.Method != "message/send" {
		return nil, errBoundary
	}
	permission, err := d.api.request(ctx, RequestCheck{Ticket: ticket, Target: resolution.Target, Protocol: "a2a", Method: resolution.Method, Params: resolution.Params})
	if err != nil || !validTicket(permission.AttemptToken) || !permission.DeadlineAt.After(time.Now()) {
		return nil, errBoundary
	}
	body, err := json.Marshal(struct {
		Version string          `json:"jsonrpc"`
		ID      string          `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}{"2.0", "authrail", resolution.Method, json.RawMessage(resolution.Params)})
	if err != nil {
		return nil, errBoundary
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.origin+"/a2a/"+resolution.Target, bytes.NewReader(body))
	if err != nil {
		return nil, errBoundary
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(dispatchHeader, d.configuration.transportToken())
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	if resolution.Method == "SendMessage" {
		req.Header.Set("A2A-Version", "1.0")
	}
	if permission.Authorization != nil {
		req.Header.Set("Authorization", *permission.Authorization)
	}
	response, err := d.client.Do(req)
	if err != nil {
		return nil, errBoundary
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, errRPC
	}
	if response.Header.Get("Content-Type") != "" && !bytes.HasPrefix([]byte(response.Header.Get("Content-Type")), []byte("application/json")) {
		return nil, errRPC
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, maxBody/2+1))
	if err != nil {
		return nil, errBoundary
	}
	if len(encoded) > maxBody/2 {
		return nil, errOversized
	}
	var reply struct {
		Version string          `json:"jsonrpc"`
		ID      string          `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	if json.Unmarshal(encoded, &reply) != nil || reply.Version != "2.0" || reply.ID != "authrail" || len(reply.Result) == 0 || len(reply.Error) != 0 {
		return nil, errRPC
	}
	if err = d.api.response(ctx, ResponseCheck{AttemptToken: permission.AttemptToken, Target: resolution.Target, Protocol: "a2a", Method: resolution.Method, Result: string(reply.Result)}); err != nil {
		return nil, err
	}
	return reply.Result, nil
}

func (d *Dispatcher) callMCP(ctx context.Context, ticket string, resolution Resolution) (json.RawMessage, error) {
	if resolution.Method != "tools/call" {
		return nil, errBoundary
	}
	// Some protocol notifications do not run ExtMCP request hooks. Supply the
	// exact ticket's account access on the private transport; the business-call
	// hook still rechecks current authority and consumes the dispatch once.
	connectionAccess, err := d.api.request(ctx, RequestCheck{Ticket: ticket, Target: resolution.Target,
		Protocol: "mcp", Method: "initialize", Params: "{}"})
	if err != nil {
		return nil, err
	}
	if err := d.checkReviewedTool(ctx, ticket, resolution, connectionAccess.Authorization); err != nil {
		return nil, err
	}
	var params mcp.CallToolParams
	decoder := json.NewDecoder(bytes.NewBufferString(resolution.Params))
	decoder.UseNumber()
	if decoder.Decode(&params) != nil || params.Name == "" {
		return nil, errBoundary
	}
	transport := &recordingTransport{Transport: &mcp.StreamableClientTransport{
		Endpoint: d.origin + "/mcp/" + resolution.Target, MaxRetries: -1, DisableStandaloneSSE: true,
		HTTPClient: &http.Client{Transport: &ticketTransport{base: http.DefaultTransport, ticket: ticket, transportToken: d.configuration.transportToken(),
			authorization: connectionAccess.Authorization, endpoint: d.origin + "/mcp/" + resolution.Target},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, params: json.RawMessage(resolution.Params)}
	client := mcp.NewClient(&mcp.Implementation{Name: "authrail-gateway-adapter", Version: "0.1.0"}, nil)
	// The pinned gateway's stateful listener uses initialize/session routing.
	// Keep its private hop on that negotiated protocol instead of the SDK's
	// newer stateless discovery default.
	session, err := client.Connect(ctx, transport, &mcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
	if err != nil {
		return nil, errRPC
	}
	defer session.Close()
	_, err = session.CallTool(ctx, &params)
	if err != nil {
		return nil, errRPC
	}
	return transport.result()
}

type ticketTransport struct {
	base                   http.RoundTripper
	ticket, transportToken string
	authorization          *string
	endpoint               string
}

func (t *ticketTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.String() != t.endpoint {
		return nil, errBoundary
	}
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set(ticketHeader, t.ticket)
	clone.Header.Set(dispatchHeader, t.transportToken)
	clone.Header.Del("Cookie")
	if t.authorization != nil {
		clone.Header.Set("Authorization", *t.authorization)
	}
	otel.GetTextMapPropagator().Inject(request.Context(), propagation.HeaderCarrier(clone.Header))
	return t.base.RoundTrip(clone)
}

// Capture raw results before the SDK decodes structured content into Go values.
// The session is scoped to one dispatch ticket; this memory is never durable truth.
type recordingTransport struct {
	mcp.Transport
	params     json.RawMessage
	connection *recordingConnection
}

func (t *recordingTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	connection, err := t.Transport.Connect(ctx)
	if err != nil {
		return nil, err
	}
	t.connection = &recordingConnection{Connection: connection, params: t.params}
	return t.connection, nil
}
func (t *recordingTransport) result() (json.RawMessage, error) {
	if t.connection == nil {
		return nil, errRPC
	}
	t.connection.mu.Lock()
	defer t.connection.mu.Unlock()
	if len(t.connection.raw) == 0 {
		return nil, errRPC
	}
	return bytes.Clone(t.connection.raw), nil
}

type recordingConnection struct {
	mcp.Connection
	mu     sync.Mutex
	params json.RawMessage
	id     jsonrpc.ID
	called bool
	raw    json.RawMessage
}

func (c *recordingConnection) Write(ctx context.Context, message jsonrpc.Message) error {
	if request, ok := message.(*jsonrpc.Request); ok && request.Method == "tools/call" {
		c.mu.Lock()
		if c.called {
			c.mu.Unlock()
			return errBoundary
		}
		c.called = true
		c.id = request.ID
		request.Params = bytes.Clone(c.params)
		c.mu.Unlock()
	}
	return c.Connection.Write(ctx, message)
}
func (c *recordingConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	message, err := c.Connection.Read(ctx)
	if err != nil {
		return nil, err
	}
	if response, ok := message.(*jsonrpc.Response); ok {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.called && response.ID == c.id && response.Error == nil {
			if len(response.Result) > maxBody/2 {
				return nil, errOversized
			}
			c.raw = bytes.Clone(response.Result)
		}
	}
	return message, nil
}
