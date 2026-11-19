package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
)

import "github.com/modelcontextprotocol/go-sdk/mcp"

const BusinessExecuteScope = "business.execute"

var businessBindingDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

type businessRequestInput struct {
	RouteID        string         `json:"routeId"`
	BindingSHA256  string         `json:"bindingSha256"`
	IdempotencyKey string         `json:"idempotencyKey"`
	Arguments      map[string]any `json:"arguments"`
}

// Keep exact JSON number tokens until the API has checked representability.
func (input *businessRequestInput) UnmarshalJSON(raw []byte) error {
	type plain businessRequestInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode((*plain)(input)); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("One exact business request is required")
	}
	return nil
}

type businessRequestReference struct {
	RequestID string `json:"requestId"`
}
type businessRequestMutation struct {
	RequestID        string `json:"requestId"`
	ExpectedRevision int    `json:"expectedRevision"`
}

func addBusinessExecutionTools(server *mcp.Server, backend *Backend) {
	uuid := map[string]any{"type": "string", "pattern": frameworkID.String()}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_request_business_action", Description: "Save and request one exact reviewed business action. Use the route and binding returned by AuthRail's reviewed-tool discovery, the reviewed tool's arguments and a UUID idempotency key reused only for identical work. AuthRail checks current authority and Rails. Waiting for approval or an unknown outcome never authorises another write.",
		InputSchema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"routeId", "bindingSha256", "idempotencyKey", "arguments"}, "properties": map[string]any{
			"routeId": uuid, "bindingSha256": map[string]any{"type": "string", "pattern": businessBindingDigest.String()}, "idempotencyKey": uuid, "arguments": map[string]any{"type": "object"},
		}},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input businessRequestInput) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RouteID) || !frameworkID.MatchString(input.IdempotencyKey) || !businessBindingDigest.MatchString(input.BindingSHA256) || input.Arguments == nil {
			return nil, nil, errors.New("Supply the exact reviewed route, binding digest, UUID idempotency key and tool arguments")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.businessRequestCall(ctx, token, "authrail_request_business_action", http.MethodPost, "/organisation/mcp/business-requests", input, identity, "")
		return nil, data, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_business_request", Description: "Read the status of an original saved business request. Only a result AuthRail has approved for release is returned. This read never dispatches or resumes work.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		InputSchema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"requestId"}, "properties": map[string]any{"requestId": uuid}},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input businessRequestReference) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.RequestID) {
			return nil, nil, errors.New("Choose the original saved request ID")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.businessRequestCall(ctx, token, "authrail_get_business_request", http.MethodGet, "/organisation/mcp/business-requests/"+input.RequestID, nil, identity, input.RequestID)
		return nil, data, err
	})
	for _, operation := range []struct{ name, path, description string }{
		{"authrail_resume_business_request", "resume", "Continue the exact saved request at its latest reviewed revision. AuthRail refreshes authority and facts; this does not grant approval or retry an unresolved provider write. No replacement arguments are accepted."},
		{"authrail_cancel_business_request", "cancel", "Request cancellation of the original saved work at its current revision. Cancellation cannot undo a dispatched effect and does not prove an external provider stopped."},
	} {
		mcp.AddTool(server, &mcp.Tool{Name: operation.name, Description: operation.description,
			InputSchema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"requestId", "expectedRevision"}, "properties": map[string]any{"requestId": uuid, "expectedRevision": map[string]any{"type": "integer", "minimum": 1}}},
		}, func(ctx context.Context, _ *mcp.CallToolRequest, input businessRequestMutation) (*mcp.CallToolResult, map[string]any, error) {
			if !frameworkID.MatchString(input.RequestID) || input.ExpectedRevision < 1 {
				return nil, nil, errors.New("Supply the original saved request ID and its latest positive revision")
			}
			token, identity := requestIdentity(ctx)
			data, err := backend.businessRequestCall(ctx, token, operation.name, http.MethodPost, "/organisation/mcp/business-requests/"+input.RequestID+"/"+operation.path,
				map[string]int{"expectedRevision": input.ExpectedRevision}, identity, input.RequestID)
			return nil, data, err
		})
	}
}

func (b *Backend) businessRequestCall(ctx context.Context, token, tool, method, path string, input any, identity Identity, expectedID string) (map[string]any, error) {
	data, err := b.delegatedCall(ctx, token, tool, method, path, BusinessExecuteScope, input, identity)
	if err != nil {
		return nil, err
	}
	safe, err := safeBusinessRequest(data)
	if err != nil || expectedID != "" && safe["requestId"] != expectedID {
		return nil, errBackend
	}
	return safe, nil
}

func safeBusinessRequest(data map[string]any) (map[string]any, error) {
	allowed := map[string]bool{"requestId": true, "revision": true, "status": true, "actionId": true, "approvalRequestId": true, "resultReleased": true, "result": true, "requiresRefresh": true, "taskState": true, "cancellationRequested": true}
	for key := range data {
		if !allowed[key] {
			return nil, errBackend
		}
	}
	requestID, _ := data["requestId"].(string)
	revision, ok := data["revision"].(float64)
	if !frameworkID.MatchString(requestID) || !ok || revision < 1 || revision > 9007199254740991 || revision != float64(int64(revision)) {
		return nil, errBackend
	}
	status, _ := data["status"].(string)
	validStatus := map[string]bool{"checking": true, "awaiting_approval": true, "ready": true, "executing": true, "checking_result": true, "succeeded": true, "failed": true, "blocked": true, "result_withheld": true, "outcome_unknown": true, "cancelled": true, "superseded": true}
	released, ok := data["resultReleased"].(bool)
	_, hasResult := data["result"]
	if !validStatus[status] || !ok || released && (status != "succeeded" || !hasResult) || !released && hasResult {
		return nil, errBackend
	}
	for _, key := range []string{"actionId", "approvalRequestId"} {
		if value, exists := data[key]; exists {
			id, ok := value.(string)
			if !ok || !frameworkID.MatchString(id) {
				return nil, errBackend
			}
		}
	}
	if value, exists := data["requiresRefresh"]; exists {
		if _, ok := value.(bool); !ok {
			return nil, errBackend
		}
	}
 if value, exists := data["cancellationRequested"]; exists { if _, ok := value.(bool); !ok { return nil, errBackend } }
 if value, exists := data["taskState"]; exists {
  state, ok := value.(string)
  states := map[string]bool{"TASK_STATE_UNSPECIFIED":true,"TASK_STATE_SUBMITTED":true,"TASK_STATE_WORKING":true,"TASK_STATE_COMPLETED":true,"TASK_STATE_FAILED":true,"TASK_STATE_CANCELED":true,"TASK_STATE_INPUT_REQUIRED":true,"TASK_STATE_REJECTED":true,"TASK_STATE_AUTH_REQUIRED":true}
  if !ok || !states[state] { return nil, errBackend }
 }
	// Keep output within the same bounded protocol envelope as the API response.
	encoded, err := json.Marshal(data)
	if err != nil || len(encoded) > maxResponse {
		return nil, errBackend
	}
	return data, nil
}
