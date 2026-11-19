package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
)

// A2ABinding is resolved from API-owned original intent/task state, never caller authority.
type A2ABinding struct {
	Version   string `json:"version"`
	MessageID string `json:"messageId,omitempty"`
	TaskID    string `json:"taskId,omitempty"`
	ContextID string `json:"contextId,omitempty"`
}
type A2AObservation struct {
	A2ABinding
	Kind           string `json:"kind"`
	State          string `json:"state,omitempty"`
	SnapshotSHA256 string `json:"snapshotSha256"`
}
type a2aPart struct {
	Text      *string         `json:"text,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	MediaType string          `json:"mediaType,omitempty"`
}
type a2aMessage struct {
	MessageID string    `json:"messageId"`
	Role      string    `json:"role"`
	Parts     []a2aPart `json:"parts"`
	TaskID    string    `json:"taskId,omitempty"`
	ContextID string    `json:"contextId,omitempty"`
}
type a2aStatus struct {
	State     string      `json:"state"`
	Message   *a2aMessage `json:"message,omitempty"`
	Timestamp string      `json:"timestamp,omitempty"`
}
type a2aArtifact struct {
	ArtifactID  string    `json:"artifactId"`
	Name        string    `json:"name,omitempty"`
	Description string    `json:"description,omitempty"`
	Parts       []a2aPart `json:"parts"`
}
type a2aTask struct {
	ID        string        `json:"id"`
	ContextID string        `json:"contextId,omitempty"`
	Status    a2aStatus     `json:"status"`
	Artifacts []a2aArtifact `json:"artifacts,omitempty"`
	History   []a2aMessage  `json:"history,omitempty"`
}

func a2aDecode(raw []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil || d.Decode(&struct{}{}) != io.EOF {
		return errBoundary
	}
	return nil
}
func a2aID(value string) bool {
	return value != "" && len(value) <= 200 && !strings.ContainsAny(value, "\r\n\x00")
}
func a2aParts(parts []a2aPart) bool {
	if len(parts) < 1 || len(parts) > 32 {
		return false
	}
	for _, p := range parts {
		if (p.Text == nil) == (len(p.Data) == 0) || (p.MediaType != "" && p.MediaType != "text/plain" && p.MediaType != "application/json") {
			return false
		}
		if p.Text != nil && len(*p.Text) > 65536 {
			return false
		}
		if len(p.Data) > 0 && !json.Valid(p.Data) {
			return false
		}
	}
	return true
}
func a2aMessageValid(m a2aMessage, role string, binding A2ABinding) bool {
	return a2aID(m.MessageID) && m.Role == role && a2aParts(m.Parts) && (m.TaskID == "" || a2aID(m.TaskID)) && (m.ContextID == "" || a2aID(m.ContextID)) && (binding.TaskID == "" || m.TaskID == binding.TaskID) && (binding.ContextID == "" || m.ContextID == binding.ContextID)
}
func validateA2ARequest(resolution Resolution) error {
	b := resolution.A2A
	if b == nil || b.Version != "1.0" {
		return errBoundary
	}
	switch resolution.Method {
	case "SendMessage":
		var p struct {
			Message       a2aMessage `json:"message"`
			Configuration struct {
				AcceptedOutputModes []string `json:"acceptedOutputModes"`
				ReturnImmediately   bool     `json:"returnImmediately"`
				HistoryLength       *int     `json:"historyLength"`
			} `json:"configuration"`
		}
		if a2aDecode([]byte(resolution.Params), &p) != nil || !a2aMessageValid(p.Message, "ROLE_USER", *b) || p.Message.MessageID != b.MessageID || p.Message.TaskID != b.TaskID || p.Message.ContextID != b.ContextID || !p.Configuration.ReturnImmediately || p.Configuration.HistoryLength == nil || *p.Configuration.HistoryLength != 0 {
			return errBoundary
		}
		// API wraps only reviewed business arguments; protocol IDs are server-owned.
		if resolution.ReviewedTool == nil || len(p.Message.Parts) != 1 || p.Message.Parts[0].MediaType != "application/json" || len(p.Message.Parts[0].Data) == 0 || ValidateReviewedSchema(resolution.ReviewedTool.InputSchema, p.Message.Parts[0].Data) != nil {
			return errBoundary
		}
		if len(p.Configuration.AcceptedOutputModes) < 1 || len(p.Configuration.AcceptedOutputModes) > 2 {
			return errBoundary
		}
		for _, mode := range p.Configuration.AcceptedOutputModes {
			if mode != "text/plain" && mode != "application/json" {
				return errBoundary
			}
		}
	case "GetTask":
		var p struct {
			ID            string `json:"id"`
			HistoryLength *int   `json:"historyLength"`
		}
		if a2aDecode([]byte(resolution.Params), &p) != nil || !a2aID(b.TaskID) || p.ID != b.TaskID || p.HistoryLength == nil || *p.HistoryLength != 0 {
			return errBoundary
		}
	case "CancelTask":
		var p struct {
			ID string `json:"id"`
		}
		if a2aDecode([]byte(resolution.Params), &p) != nil || !a2aID(b.TaskID) || p.ID != b.TaskID {
			return errBoundary
		}
	default:
		return errBoundary
	}
	return nil
}
func normalizeA2AResult(resolution Resolution, raw json.RawMessage) (*A2AObservation, error) {
	b := resolution.A2A
	if b == nil {
		return nil, errBoundary
	}
	observation := &A2AObservation{A2ABinding: *b}
	sum := sha256.Sum256(raw)
	observation.SnapshotSHA256 = hex.EncodeToString(sum[:])
	var task a2aTask
	if resolution.Method == "SendMessage" {
		var result struct {
			Task    *a2aTask    `json:"task,omitempty"`
			Message *a2aMessage `json:"message,omitempty"`
		}
		if a2aDecode(raw, &result) != nil || (result.Task == nil) == (result.Message == nil) {
			return nil, errRPC
		}
		if result.Message != nil {
			m := *result.Message
			if !a2aMessageValid(m, "ROLE_AGENT", *b) || !a2aID(m.ContextID) {
				return nil, errRPC
			}
			observation.Kind = "message"
			observation.TaskID = m.TaskID
			observation.ContextID = m.ContextID
			return observation, nil
		}
		task = *result.Task
	} else if a2aDecode(raw, &task) != nil {
		return nil, errRPC
	}
	if !a2aID(task.ID) || (b.TaskID != "" && task.ID != b.TaskID) || (b.ContextID != "" && task.ContextID != b.ContextID) || (task.ContextID != "" && !a2aID(task.ContextID)) || len(task.History) > 0 || len(task.Artifacts) > 32 {
		return nil, errRPC
	}
	switch task.Status.State {
	case "TASK_STATE_UNSPECIFIED", "TASK_STATE_SUBMITTED", "TASK_STATE_WORKING", "TASK_STATE_COMPLETED", "TASK_STATE_FAILED", "TASK_STATE_CANCELED", "TASK_STATE_INPUT_REQUIRED", "TASK_STATE_REJECTED", "TASK_STATE_AUTH_REQUIRED":
	default:
		return nil, errRPC
	}
	if task.Status.Timestamp != "" {
		if _, err := time.Parse(time.RFC3339Nano, task.Status.Timestamp); err != nil {
			return nil, errRPC
		}
	}
	bound := A2ABinding{TaskID: task.ID, ContextID: task.ContextID}
	if task.Status.Message != nil && !a2aMessageValid(*task.Status.Message, "ROLE_AGENT", bound) {
		return nil, errRPC
	}
	ids := map[string]bool{}
	for _, artifact := range task.Artifacts {
		if !a2aID(artifact.ArtifactID) || ids[artifact.ArtifactID] || !a2aParts(artifact.Parts) || len(artifact.Name) > 1000 || len(artifact.Description) > 10000 {
			return nil, errRPC
		}
		ids[artifact.ArtifactID] = true
	}
	observation.Kind = "task"
	observation.TaskID = task.ID
	observation.ContextID = task.ContextID
	observation.State = task.Status.State
	return observation, nil
}
func (d *Dispatcher) callA2A(ctx context.Context, ticket string, resolution Resolution) (json.RawMessage, error) {
	if err := validateA2ARequest(resolution); err != nil {
		return nil, err
	}
	permission, err := d.api.request(ctx, RequestCheck{Ticket: ticket, Target: resolution.Target, Protocol: "a2a", Method: resolution.Method, Params: resolution.Params})
	if err != nil || !validTicket(permission.AttemptToken) || !permission.DeadlineAt.After(time.Now()) {
		return nil, errBoundary
	}
	callCtx, cancel := context.WithDeadline(ctx, permission.DeadlineAt)
	defer cancel()
	body, err := json.Marshal(struct {
		Version string          `json:"jsonrpc"`
		ID      string          `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}{"2.0", "authrail", resolution.Method, json.RawMessage(resolution.Params)})
	if err != nil {
		return nil, errBoundary
	}
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, d.origin+"/a2a/"+resolution.Target, bytes.NewReader(body))
	if err != nil {
		return nil, errBoundary
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("A2A-Version", "1.0")
	req.Header.Set(dispatchHeader, d.configuration.transportToken())
	if permission.Authorization != nil {
		req.Header.Set("Authorization", *permission.Authorization)
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	response, err := d.client.Do(req)
	if err != nil {
		return nil, errBoundary
	}
	defer response.Body.Close()
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || response.StatusCode < 200 || response.StatusCode >= 300 {
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
		Error   json.RawMessage `json:"error,omitempty"`
	}
	if a2aDecode(encoded, &reply) != nil || reply.Version != "2.0" || reply.ID != "authrail" || len(reply.Result) == 0 || len(reply.Error) != 0 {
		return nil, errRPC
	}
	observation, err := normalizeA2AResult(resolution, reply.Result)
	if err != nil {
		return nil, err
	}
	resultForReview := reply.Result
	if resolution.Method != "SendMessage" {
		resultForReview = append(append([]byte(`{"task":`), reply.Result...), '}')
	}
	contractValid := resolution.ReviewedTool != nil && ValidateReviewedSchema(resolution.ReviewedTool.OutputSchema, resultForReview) == nil
	// Retain a known task even if the caller disconnected after the provider replied.
	retainCtx, retainCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer retainCancel()
	if err = d.api.response(retainCtx, ResponseCheck{AttemptToken: permission.AttemptToken, Target: resolution.Target, Protocol: "a2a", Method: resolution.Method, Result: string(reply.Result), A2A: observation, ReviewedContractValid: &contractValid}); err != nil {
		return nil, err
	}
	if !contractValid {
		return nil, errRPC
	}
	return reply.Result, nil
}
