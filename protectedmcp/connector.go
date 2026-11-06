// Package protectedmcp applies the AuthRail Go adapter to explicitly reviewed customer MCP tools.
package protectedmcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	authrail "github.com/AuthRail/authrail-sdk-go"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolClient is the authenticated downstream session; implementations must not retry tool calls.
type ToolClient interface {
	ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error)
	CallTool(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error)
}

// Binding pins the downstream contract; descriptions and annotations alone never establish permission.
type Binding struct {
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	DownstreamName     string            `json:"downstreamName"`
	OperationVersionID string            `json:"operationVersionId"`
	SchemaSHA256       string            `json:"schemaSha256"`
	ReadOnly           bool              `json:"readOnly"`
	IdempotencyField   string            `json:"idempotencyField,omitempty"`
	ResourceField      string            `json:"resourceField,omitempty"`
	Resources          map[string]string `json:"resources,omitempty"`
	ReviewFields       []string          `json:"reviewFields,omitempty"`
}

// Config is supplied by the credential-owning operator, never by MCP tool arguments.
type Config struct {
	Runtime        authrail.Config
	PrincipalID    string
	SessionID      string
	StateDirectory string
	JournalKey     []byte
	Bindings       []Binding
	Downstream     ToolClient
	// ResolveResource is optional trusted application lookup, not an agent-written callback.
	ResolveResource func(context.Context, Binding, map[string]any) (string, error)
}
type tool struct {
	binding         Binding
	inputSchema     map[string]any
	validator       *jsonschema.Resolved
	pin             string
	outputValidator *jsonschema.Resolved
}

// Connector owns one trusted identity and its private execution journal.
type Connector struct {
	config  Config
	runtime *authrail.Client
	state   *store
	tools   map[string]tool
}
type invocation struct {
	RequestKey string         `json:"requestKey"`
	Arguments  map[string]any `json:"arguments"`
}

// ToolResult is the result contract reviewed by the AuthRail Operation.
type ToolResult struct {
	Text string         `json:"text"`
	Data map[string]any `json:"data"`
}
type reply struct {
	Status       string      `json:"status"`
	DeploymentID string      `json:"deploymentId"`
	ActionID     string      `json:"actionId,omitempty"`
	ApprovalID   string      `json:"approvalRequestId,omitempty"`
	Result       *ToolResult `json:"result,omitempty"`
}

var identifier = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
var downstreamName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
var requestKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var uuid = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var digest = regexp.MustCompile(`^[a-f0-9]{64}$`)

func hashJSON(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

// SchemaDigest pins both input and output contracts without trusting tool annotations.
func SchemaDigest(value *mcp.Tool) (string, error) {
	return hashJSON(struct {
		Input  any `json:"input"`
		Output any `json:"output"`
	}{value.InputSchema, value.OutputSchema})
}

// New verifies fixed tool contracts before exposing any protected operation.
func New(ctx context.Context, config Config) (*Connector, error) {
	if config.Downstream == nil || !requestKey.MatchString(config.PrincipalID) || !requestKey.MatchString(config.SessionID) || len(config.Bindings) == 0 || len(config.Bindings) > 64 {
		return nil, errors.New("fixed application identity and reviewed tool bindings are required")
	}
	client, err := authrail.NewClient(config.Runtime)
	if err != nil {
		return nil, err
	}
	state, err := openStore(config.StateDirectory, config.JournalKey)
	if err != nil {
		return nil, err
	}
	c := &Connector{config: config, runtime: client, state: state, tools: map[string]tool{}}
	if err = c.loadBindings(ctx); err != nil {
		state.close()
		return nil, err
	}
	return c, nil
}

// Close releases exclusive journal ownership after callers have stopped sending requests.
func (c *Connector) Close() error { return c.state.close() }
func discoverTools(ctx context.Context, downstream ToolClient) (map[string]*mcp.Tool, error) {
	available := map[string]*mcp.Tool{}
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < 10; page++ {
		result, err := downstream.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil || result == nil {
			return nil, errors.New("downstream tool discovery failed")
		}
		for _, item := range result.Tools {
			if item == nil || available[item.Name] != nil || len(available) >= 256 {
				return nil, errors.New("downstream discovery is ambiguous or exceeds its bound")
			}
			available[item.Name] = item
		}
		if result.NextCursor == "" {
			break
		}
		if seen[result.NextCursor] || page == 9 {
			return nil, errors.New("downstream discovery cannot be completed")
		}
		seen[result.NextCursor] = true
		cursor = result.NextCursor
	}
	return available, nil
}

// ReviewedInputSchema excludes only the idempotency value supplied by the protected connector.
func ReviewedInputSchema(source any, idempotencyField string) (map[string]any, error) {
	encoded, err := json.Marshal(source)
	if err != nil || len(encoded) > 32768 {
		return nil, errors.New("downstream input schema exceeds its bound")
	}
	var schema map[string]any
	if json.Unmarshal(encoded, &schema) != nil || schema["type"] != "object" {
		return nil, errors.New("downstream tool needs an object input schema")
	}
	if idempotencyField != "" {
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			return nil, errors.New("idempotency property is absent")
		}
		property, ok := props[idempotencyField].(map[string]any)
		if !ok || property["type"] != "string" {
			return nil, errors.New("reviewed idempotency field must be a string")
		}
		delete(props, idempotencyField)
		required, _ := schema["required"].([]any)
		filtered := []any{}
		for _, field := range required {
			if field != idempotencyField {
				filtered = append(filtered, field)
			}
		}
		schema["required"] = filtered
	}
	return schema, nil
}

// InspectTools lists bounded downstream contracts for review without calling business tools.
func InspectTools(ctx context.Context, downstream ToolClient) ([]*mcp.Tool, error) {
	available, err := discoverTools(ctx, downstream)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(available))
	for name := range available {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]*mcp.Tool, 0, len(names))
	for _, name := range names {
		result = append(result, available[name])
	}
	return result, nil
}
func (c *Connector) loadBindings(ctx context.Context) error {
	available, err := discoverTools(ctx, c.config.Downstream)
	if err != nil {
		return err
	}
	for _, binding := range c.config.Bindings {
		if !identifier.MatchString(binding.Name) || strings.HasPrefix(binding.Name, "authrail_") || !downstreamName.MatchString(binding.DownstreamName) || !uuid.MatchString(binding.OperationVersionID) || !digest.MatchString(binding.SchemaSHA256) || c.tools[binding.Name].binding.Name != "" || len(binding.Description) > 500 || len(binding.Resources) > 10000 || len(binding.ReviewFields) > 8 {
			return errors.New("reviewed tool binding is invalid")
		}
		if !binding.ReadOnly && (!identifier.MatchString(binding.IdempotencyField) || binding.ResourceField == "" || len(binding.ReviewFields) == 0) {
			return errors.New("write tools require a reviewed idempotency field and trusted resource binding")
		}
		if binding.ResourceField != "" && !identifier.MatchString(binding.ResourceField) {
			return errors.New("resource field is invalid")
		}
		for reference, id := range binding.Resources {
			if !requestKey.MatchString(reference) || !requestKey.MatchString(id) {
				return errors.New("trusted resource mapping is invalid")
			}
		}
		if !binding.ReadOnly {
			includesResource := false
			for _, field := range binding.ReviewFields {
				if !identifier.MatchString(field) {
					return errors.New("review fields must name exact input properties")
				}
				includesResource = includesResource || field == binding.ResourceField
			}
			if !includesResource {
				return errors.New("write approval context must identify its resource")
			}
		}
		actual := available[binding.DownstreamName]
		if actual == nil {
			return errors.New("a reviewed downstream tool is missing")
		}
		pin, err := SchemaDigest(actual)
		if err != nil || pin != binding.SchemaSHA256 {
			return errors.New("downstream tool contract changed; review the binding")
		}
		schema, err := ReviewedInputSchema(actual.InputSchema, binding.IdempotencyField)
		if err != nil {
			return err
		}
		encoded, _ := json.Marshal(schema)
		var input jsonschema.Schema
		if json.Unmarshal(encoded, &input) != nil {
			return errors.New("input schema is unsupported")
		}
		validator, err := input.Resolve(nil)
		if err != nil {
			return errors.New("input schema references cannot be resolved locally")
		}
		frozen, _ := json.Marshal(binding)
		var fixed Binding
		_ = json.Unmarshal(frozen, &fixed)
		bindingPin, _ := hashJSON(fixed)
		var outputValidator *jsonschema.Resolved
		if actual.OutputSchema != nil {
			raw, marshalErr := json.Marshal(actual.OutputSchema)
			if marshalErr != nil || len(raw) > 32768 {
				return errors.New("downstream output schema exceeds its bound")
			}
			var output jsonschema.Schema
			if json.Unmarshal(raw, &output) != nil {
				return errors.New("downstream output schema is invalid")
			}
			outputValidator, err = output.Resolve(nil)
			if err != nil {
				return errors.New("downstream output references cannot be resolved locally")
			}
		}
		c.tools[binding.Name] = tool{binding: fixed, inputSchema: schema, validator: validator, pin: bindingPin, outputValidator: outputValidator}
	}
	return nil
}

// Server exposes only reviewed business tools and their saved approval continuation.
func (c *Connector) Server() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "authrail-protected-mcp", Version: "0.1.0"}, nil)
	for name, bound := range c.tools {
		schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"requestKey", "arguments"}, "properties": map[string]any{"requestKey": map[string]any{"type": "string", "pattern": requestKey.String(), "description": "Stable key for this business intent. Reuse for the same request; never create a new key to retry an uncertain effect."}, "arguments": publicInputSchema(bound.inputSchema)}}
		server.AddTool(&mcp.Tool{Name: name, Description: bound.binding.Description + " AuthRail checks this exact tool. Send a stable requestKey and its arguments; a denied call does not execute, and approval is permission rather than proof of completion.", InputSchema: schema}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if request.Params == nil || len(request.Params.Arguments) > 65536 || len(request.Params.InputResponses) > 0 || request.Params.RequestState != "" {
				return nil, errors.New("a bounded new protected request is required")
			}
			var input invocation
			var shape map[string]json.RawMessage
			if json.Unmarshal(request.Params.Arguments, &shape) != nil || len(shape) != 2 || shape["requestKey"] == nil || shape["arguments"] == nil {
				return nil, errors.New("use exact requestKey and arguments fields")
			}
			if err := decodeExact(request.Params.Arguments, &input); err != nil {
				return nil, errors.New("use requestKey and arguments")
			}
			result, err := c.invoke(ctx, name, input)
			if err != nil {
				return nil, errors.New("protected request could not be completed; inspect the existing request before retrying")
			}
			return output(result), nil
		})
	}
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_continue_request", Description: "Check a saved request after human review. This cannot approve or change its arguments. Pending requests stay saved; ambiguous effects are never replayed."}, func(ctx context.Context, _ *mcp.CallToolRequest, input struct {
		Tool       string `json:"tool"`
		RequestKey string `json:"requestKey"`
	}) (*mcp.CallToolResult, reply, error) {
		result, err := c.resume(ctx, input.Tool, input.RequestKey)
		if err != nil {
			return nil, reply{}, errors.New("saved request could not be continued; inspect its AuthRail Action")
		}
		return nil, result, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_saved_request", Description: "Read the local status and AuthRail Action ID for your saved business request. This does not execute, approve, or return previously withheld output.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, _ *mcp.CallToolRequest, input struct {
		Tool       string `json:"tool"`
		RequestKey string `json:"requestKey"`
	}) (*mcp.CallToolResult, reply, error) {
		bound, ok := c.tools[input.Tool]
		if !ok || !requestKey.MatchString(input.RequestKey) {
			return nil, reply{}, errors.New("saved request is invalid")
		}
		key := c.key(input.Tool, input.RequestKey)
		release := c.state.acquire(key)
		defer release()
		saved, err := c.state.load(key)
		if err != nil || saved == nil || saved.Binding != bound.pin {
			return nil, reply{}, errors.New("saved request is unavailable")
		}
		return nil, c.publicRecord(saved), nil
	})

	return server
}
func output(value reply) *mcp.CallToolResult {
	body, _ := json.Marshal(value)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}, StructuredContent: value}
}
func decodeExact(body []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("trailing input")
	}
	return nil
}
func (c *Connector) key(name, key string) string {
	return stateKey(c.config.Runtime.OrganisationID + ":" + c.config.Runtime.DeploymentID + ":" + c.config.PrincipalID + ":" + name + ":" + key)
}
func (c *Connector) publicRecord(value *record) reply {
	return reply{DeploymentID: c.config.Runtime.DeploymentID, Status: value.State, ActionID: value.ActionID, ApprovalID: value.ApprovalID}
}
func (c *Connector) invoke(ctx context.Context, name string, input invocation) (reply, error) {
	bound, ok := c.tools[name]
	if !ok || !requestKey.MatchString(input.RequestKey) || input.Arguments == nil {
		return reply{}, errors.New("invalid protected request")
	}
	encoded, err := json.Marshal(input.Arguments)
	if err != nil || len(encoded) > 65536 {
		return reply{}, errors.New("arguments exceed their bound")
	}
	var args map[string]any
	_ = json.Unmarshal(encoded, &args)
	if bound.validator.Validate(args) != nil {
		return reply{}, errors.New("arguments do not match the reviewed tool schema")
	}
	if _, exists := args[bound.binding.IdempotencyField]; bound.binding.IdempotencyField != "" && exists {
		return reply{}, errors.New("the application owns idempotency")
	}
	key := c.key(name, input.RequestKey)
	release := c.state.acquire(key)
	defer release()
	intent, _ := hashJSON(args)
	saved, err := c.state.load(key)
	if err != nil {
		return reply{}, err
	}
	if saved != nil {
		if saved.Intent != intent || saved.Binding != bound.pin {
			return reply{}, errors.New("request key or binding changed")
		}
		return c.publicRecord(saved), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	resource, err := c.resolveResource(ctx, bound.binding, args)
	if err != nil {
		return reply{}, err
	}
	reference, _ := args[bound.binding.ResourceField].(string)
	saved = &record{Binding: bound.pin, Intent: intent, State: "checking", ResourceID: resource, ResourceReference: reference}
	if err = c.state.save(key, saved); err != nil {
		return reply{}, err
	}
	operation := authrail.Operation{VersionID: bound.binding.OperationVersionID, ActionKey: "mcp-" + key, PrincipalID: c.config.PrincipalID, SessionID: c.config.SessionID, PrincipalSource: "service_identity", ResourceID: resource, ResourceSource: "not_applicable"}
	if resource != "" {
		operation.ResourceSource = "application_lookup"
	}
	operation.ReviewSummary = reviewSummary(bound.binding, args)
	result, decision, _, err := authrail.Enforce[map[string]any, ToolResult](ctx, c.runtime, operation, args, c.executor(bound, key, saved))
	return c.finish(key, saved, result, decision, err)
}
func (c *Connector) resolveResource(ctx context.Context, binding Binding, args map[string]any) (string, error) {
	if binding.ResourceField == "" {
		return "", nil
	}
	reference, ok := args[binding.ResourceField].(string)
	if !ok || !requestKey.MatchString(reference) {
		return "", errors.New("trusted resource reference is unavailable")
	}
	resource := ""
	var err error
	if c.config.ResolveResource != nil {
		resource, err = c.config.ResolveResource(ctx, binding, args)
	} else {
		resource = binding.Resources[reference]
	}
	if err != nil || !requestKey.MatchString(resource) {
		return "", errors.New("trusted resource binding is unavailable")
	}
	return resource, nil
}
func reviewSummary(binding Binding, args map[string]any) *authrail.ReviewSummary {
	fields := []authrail.ReviewField{}
	for _, name := range binding.ReviewFields {
		value, ok := args[name]
		if !ok {
			continue
		}
		switch value.(type) {
		case string, float64, bool:
			text := fmt.Sprint(value)
			if len(text) > 200 {
				text = text[:200]
			}
			fields = append(fields, authrail.ReviewField{Label: name, Value: text})
		}
	}
	if len(fields) == 0 {
		fields = append(fields, authrail.ReviewField{Label: "Tool", Value: binding.Name})
	}
	return &authrail.ReviewSummary{Title: "Review " + binding.Name, Fields: fields}
}
func (c *Connector) executor(bound tool, key string, saved *record) func(context.Context, map[string]any, authrail.ExecutionContext) (ToolResult, error) {
	return func(ctx context.Context, args map[string]any, execution authrail.ExecutionContext) (ToolResult, error) {
		// This durable marker precedes every downstream effect, including approval recovery.
		saved.State = "started"
		saved.ActionID = execution.ActionID
		recovery := authrail.Recovery(execution)
		saved.Recovery = &recovery
		if err := c.state.save(key, saved); err != nil {
			return ToolResult{}, err
		}
		// Guards cannot redirect an authorised request to a different resource, and ownership is checked again after review.
		reference, _ := args[bound.binding.ResourceField].(string)
		resource, err := c.resolveResource(ctx, bound.binding, args)
		if err != nil || resource != saved.ResourceID || reference != saved.ResourceReference || bound.validator.Validate(args) != nil {
			return ToolResult{}, errors.New("protected resource or input changed")
		}
		available, err := discoverTools(ctx, c.config.Downstream)
		if err != nil {
			return ToolResult{}, errors.New("downstream contract cannot be checked")
		}
		actual := available[bound.binding.DownstreamName]
		if actual == nil {
			return ToolResult{}, errors.New("downstream tool is missing")
		}
		pin, err := SchemaDigest(actual)
		if err != nil || pin != bound.binding.SchemaSHA256 {
			return ToolResult{}, errors.New("downstream tool contract changed")
		}

		if bound.binding.IdempotencyField != "" {
			args[bound.binding.IdempotencyField] = execution.ProviderIdempotencyKey
		}
		result, err := c.config.Downstream.CallTool(ctx, &mcp.CallToolParams{Name: bound.binding.DownstreamName, Arguments: args})
		if err != nil || result == nil || result.IsError || result.NeedsInput() || result.RequestState != "" {
			return ToolResult{}, errors.New("downstream outcome must be inspected")
		}
		if bound.outputValidator != nil && bound.outputValidator.Validate(result.StructuredContent) != nil {
			return ToolResult{}, errors.New("downstream output did not match its reviewed schema")
		}
		text := []string{}
		for _, content := range result.Content {
			part, ok := content.(*mcp.TextContent)
			if !ok {
				return ToolResult{}, errors.New("only reviewed text and structured results can be released")
			}
			text = append(text, part.Text)
		}
		data := map[string]any{}
		if result.StructuredContent != nil {
			body, err := json.Marshal(result.StructuredContent)
			if err != nil || json.Unmarshal(body, &data) != nil || data == nil {
				return ToolResult{}, errors.New("structured result must be an object")
			}
		}
		value := ToolResult{Text: strings.Join(text, "\n"), Data: data}
		body, err := json.Marshal(value)
		if err != nil || len(body) > 65536 {
			return ToolResult{}, errors.New("result exceeds its reviewed bound")
		}
		return value, nil
	}
}
func (c *Connector) finish(key string, saved *record, result ToolResult, decision *authrail.Decision, err error) (reply, error) {
	if decision != nil {
		saved.ActionID = decision.ActionID
		saved.ApprovalID = decision.ApprovalRequestID
	}
	var pending *authrail.ApprovalRequired
	var unknown *authrail.OutcomeUnknownError
	var blocked *authrail.EnforceBlockedError
	switch {
	case errors.As(err, &pending):
		continuation, failure := pending.Continuation()
		if failure != nil {
			return reply{}, failure
		}
		saved.Continuation = &continuation
		saved.ActionID = pending.Decision.ActionID
		saved.ApprovalID = pending.Decision.ApprovalRequestID
		saved.State = "awaiting_approval"
	case errors.As(err, &unknown):
		saved.State = "outcome_unknown"
		saved.Recovery = &unknown.Recovery
		saved.ActionID = unknown.Recovery.ActionID
	case errors.As(err, &blocked):
		if blocked.Decision != nil {
			saved.ActionID = blocked.Decision.ActionID
		}
		if blocked.EffectMayHaveOccurred {
			saved.State = "result_withheld"
		} else if saved.State == "started" {
			saved.State = "outcome_unknown"
		} else {
			saved.State = "blocked"
		}
	case err != nil:
		saved.State = "outcome_unknown"
	default:
		saved.State = "completed"
	}
	if failure := c.state.save(key, saved); failure != nil {
		return reply{}, failure
	}
	response := c.publicRecord(saved)
	if err == nil {
		response.Result = &result
	}
	return response, nil
}

// Rebase local references when an existing tool schema becomes the arguments child.
func publicInputSchema(value map[string]any) map[string]any {
	encoded, _ := json.Marshal(value)
	var result map[string]any
	_ = json.Unmarshal(encoded, &result)
	var visit func(any)
	visit = func(node any) {
		switch item := node.(type) {
		case map[string]any:
			for key, child := range item {
				if key == "$ref" {
					if reference, ok := child.(string); ok {
						if reference == "#" {
							item[key] = "#/properties/arguments"
						} else if strings.HasPrefix(reference, "#/") {
							item[key] = "#/properties/arguments/" + reference[2:]
						}
					}
				} else {
					visit(child)
				}
			}
		case []any:
			for _, child := range item {
				visit(child)
			}
		}
	}
	visit(result)
	return result
}
