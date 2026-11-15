package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/AuthRail/authrail-mcp/internal/gateway/wire"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/protobuf/types/known/structpb"
)

type Hooks struct {
	wire.UnimplementedExtMcpServer
	API *API
}

func denied() *wire.AuthorizationError {
	return &wire.AuthorizationError{Code: wire.AuthorizationError_PERMISSION_DENIED, Reason: "Account action is not available"}
}

func (h *Hooks) CheckRequest(ctx context.Context, request *wire.McpRequest) (*wire.McpRequestResult, error) {
	fail := func() (*wire.McpRequestResult, error) {
		return &wire.McpRequestResult{
			Result: &wire.McpRequestResult_Error{Error: denied()}}, nil
	}
	if len(request.ServiceNames) != 1 || !validTarget(request.ServiceNames[0]) || len(request.McpRequest) > maxBody/2 {
		return fail()
	}
	if request.Method != "server/discover" && request.Method != "initialize" && request.Method != "notifications/initialized" &&
		request.Method != "tools/list" && request.Method != "tools/call" {
		return fail()
	}
	ticket := ""
	carrier := http.Header{}
	for _, header := range request.Headers {
		if (strings.EqualFold(header.Key, "traceparent") || strings.EqualFold(header.Key, "tracestate")) && len(header.Value) < 512 {
			carrier.Set(header.Key, string(header.Value))
		}
		if strings.EqualFold(header.Key, ticketHeader) {
			if ticket != "" || !validTicket(string(header.Value)) {
				return fail()
			}
			ticket = string(header.Value)
		}
	}
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(carrier))
	if ticket == "" {
		return fail()
	}
	params := request.McpRequest
	if len(params) == 0 {
		params = []byte("{}")
	}
	if !json.Valid(params) {
		return fail()
	}
	check := RequestCheck{Ticket: ticket, Target: request.ServiceNames[0], Protocol: "mcp", Method: request.Method, Params: string(params)}
	if request.MetadataContext != nil {
		check.SessionID = metadataText(request.MetadataContext, "sessionId")
	}
	permission, err := h.API.request(ctx, check)
	if err != nil || !permission.DeadlineAt.After(time.Now()) {
		return fail()
	}
	if request.Method == "tools/call" && !validTicket(permission.AttemptToken) {
		return fail()
	}
	mutation := &wire.HeaderMutation{Remove: []string{ticketHeader, "Authorization", "Cookie", "X-Organisation-Id"}}
	if permission.Authorization != nil {
		// Set/remove ordering is gateway-defined; do not remove the newly set value.
		mutation.Remove = []string{ticketHeader, "Cookie", "X-Organisation-Id"}
		mutation.Set = []*wire.McpHeader{{Key: "Authorization", Value: []byte(*permission.Authorization)}}
	}
	propagated := http.Header{}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(propagated))
	metadata, err := structpb.NewStruct(map[string]interface{}{"attemptToken": permission.AttemptToken, "traceparent": propagated.Get("traceparent"), "tracestate": propagated.Get("tracestate")})
	if err != nil {
		return fail()
	}
	return &wire.McpRequestResult{Result: &wire.McpRequestResult_Pass{Pass: &wire.Pass{}},
		HeaderMutation: mutation, Metadata: metadata}, nil
}

func (h *Hooks) CheckResponse(ctx context.Context, response *wire.McpResponse) (*wire.McpResponseResult, error) {
	fail := func() (*wire.McpResponseResult, error) {
		return &wire.McpResponseResult{
			Result: &wire.McpResponseResult_Error{Error: denied()}}, nil
	}
	if response.Method != "tools/call" || len(response.ServiceNames) != 1 ||
		!validTarget(response.ServiceNames[0]) || len(response.McpResponse) > maxBody/2 || !json.Valid(response.McpResponse) {
		return fail()
	}
	attempt := metadataText(response.MetadataContext, "attemptToken")
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(http.Header{
		"Traceparent": []string{metadataText(response.MetadataContext, "traceparent")},
		"Tracestate":  []string{metadataText(response.MetadataContext, "tracestate")},
	}))
	if !validTicket(attempt) {
		return fail()
	}
	if err := h.API.response(ctx, ResponseCheck{AttemptToken: attempt, Target: response.ServiceNames[0],
		Protocol: "mcp", Method: response.Method, Result: string(response.McpResponse)}); err != nil {
		return fail()
	}
	return &wire.McpResponseResult{Result: &wire.McpResponseResult_Pass{Pass: &wire.Pass{}}}, nil
}

func metadataText(metadata *structpb.Struct, key string) string {
	if metadata == nil {
		return ""
	}
	value := metadata.Fields[key]
	if value == nil {
		return ""
	}
	return value.GetStringValue()
}
