package protectedmcp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	authrail "github.com/AuthRail/authrail-sdk-go"
)

// Status proofs use a separate purpose and cannot be replayed as permit claims.
func (c *Connector) approvalState(ctx context.Context, value authrail.ApprovalContinuation) (string, error) {
	call := value.Call
	var args map[string]any
	if json.Unmarshal(call.Arguments, &args) != nil {
		return "", errors.New("saved approval arguments are invalid")
	}
	evidence := []authrail.GuardEvidence{}
	for _, item := range call.GuardEvidence {
		if item.Phase == "tool_call" {
			evidence = append(evidence, item)
		}
	}
	// Preserve the Go adapter's language-neutral call digest, including original facts.
	input, err := json.Marshal(struct {
		Arguments  map[string]any               `json:"arguments"`
		Facts      []authrail.FactResolution    `json:"factResolutions,omitempty"`
		Trajectory *authrail.TrajectorySnapshot `json:"trajectory,omitempty"`
		Evidence   []authrail.GuardEvidence     `json:"guardEvidence,omitempty"`
	}{args, call.FactResolutions, call.Trajectory, evidence})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(input)
	inputHash := hex.EncodeToString(sum[:])
	key := c.config.Runtime.ExecutorPrivateKey
	fingerprint := sha256.Sum256(key.Public().(ed25519.PublicKey))
	keyHash := hex.EncodeToString(fingerprint[:])
	now := time.Now().Unix()
	statement := struct {
		Type              string `json:"type"`
		OrganisationID    string `json:"organisationId"`
		DeploymentID      string `json:"deploymentId"`
		ApprovalRequestID string `json:"approvalRequestId"`
		ActionID          string `json:"actionId"`
		InputSHA256       string `json:"inputSha256"`
		KeySHA256         string `json:"keySha256"`
		IssuedAt          int64  `json:"issuedAt"`
		ExpiresAt         int64  `json:"expiresAt"`
	}{"authrail.approval.status.v1", c.config.Runtime.OrganisationID, c.config.Runtime.DeploymentID, value.Decision.ApprovalRequestID, value.Decision.ActionID, inputHash, keyHash, now, now + 30}
	encoded, _ := json.Marshal(statement)
	body, _ := json.Marshal(map[string]any{"approvalRequestId": statement.ApprovalRequestID, "actionId": statement.ActionID, "inputSha256": inputHash, "keySha256": keyHash, "proof": map[string]any{"issuedAt": now, "expiresAt": now + 30, "signature": base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, encoded))}})
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.config.Runtime.SidecarURL, "/")+"/approval-permits/status", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+c.config.Runtime.AdapterToken)
	request.Header.Set("Content-Type", "application/json")
	client := c.config.Runtime.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 4 * time.Second}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := copyClient.Do(request)
	if err != nil {
		return "", errors.New("approval status is unavailable")
	}
	defer response.Body.Close()
	var status struct {
		ActionID          string    `json:"actionId"`
		ApprovalRequestID string    `json:"approvalRequestId"`
		State             string    `json:"state"`
		ExpiresAt         time.Time `json:"expiresAt"`
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2049))
	if err != nil || len(raw) > 2048 || response.StatusCode != 200 || json.Unmarshal(raw, &status) != nil || status.ActionID != value.Decision.ActionID || status.ApprovalRequestID != value.Decision.ApprovalRequestID {
		return "", errors.New("approval status is invalid")
	}
	switch status.State {
	case "pending", "approved", "rejected", "cancelled", "expired", "consumed":
	default:
		return "", errors.New("approval status is unsupported")
	}
	if !status.ExpiresAt.After(time.Now()) && status.State != "consumed" {
		return "expired", nil
	}
	return status.State, nil
}
func (c *Connector) resume(ctx context.Context, name, requestKeyValue string) (reply, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	bound, ok := c.tools[name]
	if !ok || !requestKey.MatchString(requestKeyValue) {
		return reply{}, errors.New("saved request is invalid")
	}
	key := c.key(name, requestKeyValue)
	release := c.state.acquire(key)
	defer release()
	saved, err := c.state.load(key)
	if err != nil || saved == nil {
		return reply{}, errors.New("saved request is unavailable")
	}
	if saved.Binding != bound.pin {
		return reply{}, errors.New("tool binding changed; the original request requires operator review")
	}
	if saved.State != "awaiting_approval" && saved.State != "claim_unknown" {
		return c.publicRecord(saved), nil
	}
	continuation := saved.Continuation
	if continuation == nil || continuation.Call.OperationVersionID != bound.binding.OperationVersionID || continuation.Call.PrincipalID != c.config.PrincipalID || continuation.Call.SessionID != c.config.SessionID {
		return reply{}, errors.New("saved approval binding is invalid")
	}
	state, err := c.approvalState(ctx, *continuation)
	if err != nil {
		return reply{}, err
	}
	if state == "pending" {
		saved.State = "awaiting_approval"
		if err = c.state.save(key, saved); err != nil {
			return reply{}, err
		}
		return c.publicRecord(saved), nil
	}
	if state != "approved" && state != "consumed" {
		saved.State = state
		if err = c.state.save(key, saved); err != nil {
			return reply{}, err
		}
		return c.publicRecord(saved), nil
	}
	// An exact consumed request can only recover its existing unstarted permit.
	saved.State = "claim_unknown"
	if err = c.state.save(key, saved); err != nil {
		return reply{}, err
	}
	var result ToolResult
	var decision *authrail.Decision
	if state == "consumed" {
		result, decision, err = authrail.RecoverApprovalFrom[map[string]any, ToolResult](ctx, c.runtime, *continuation, c.executor(bound, key, saved))
	} else {
		result, decision, err = authrail.ResumeApprovalFrom[map[string]any, ToolResult](ctx, c.runtime, *continuation, c.executor(bound, key, saved))
	}
	if err != nil && saved.State == "claim_unknown" {
		return c.publicRecord(saved), nil
	}
	return c.finish(key, saved, result, decision, err)
}

// Reconcile records a provider outcome established by trusted application code.
// It never releases a saved result or runs the business tool again. Do not expose the lookup to agent arguments.
func (c *Connector) Reconcile(ctx context.Context, name, keyValue string, lookup func(context.Context, authrail.Recovery) (string, string, error)) error {
	bound, ok := c.tools[name]
	if !ok || !requestKey.MatchString(keyValue) || lookup == nil {
		return errors.New("trusted reconciliation is required")
	}
	key := c.key(name, keyValue)
	release := c.state.acquire(key)
	defer release()
	saved, err := c.state.load(key)
	if err != nil || saved == nil || saved.Binding != bound.pin || saved.Recovery == nil {
		return errors.New("recovery record is unavailable")
	}
	if saved.State != "outcome_unknown" && saved.State != "started" {
		return errors.New("this request does not need provider reconciliation")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	reference, outcome, err := lookup(ctx, *saved.Recovery)
	if err != nil {
		return errors.New("provider outcome remains uncertain")
	}
	if _, err = c.runtime.Reconcile(ctx, *saved.Recovery, reference, outcome); err != nil {
		return errors.New("provider reconciliation was not retained")
	}
	saved.State = "reconciled_" + outcome
	return c.state.save(key, saved)
}
