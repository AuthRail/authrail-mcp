package protectedmcp

import (
	"encoding/json"
	"errors"
	"path/filepath"

	authrail "github.com/AuthRail/authrail-sdk-go"
)

// OperatorRequestStatus is private operator diagnostics, never a business result
// or permission to retry. Recovery identifies original work for trusted lookup.
type OperatorRequestStatus struct {
	OrganisationID           string             `json:"organisationId"`
	DeploymentID             string             `json:"deploymentId"`
	PrincipalID              string             `json:"principalId"`
	SessionID                string             `json:"sessionId"`
	Tool                     string             `json:"tool"`
	RequestKey               string             `json:"requestKey"`
	Status                   string             `json:"status"`
	SavedState               string             `json:"savedState"`
	CheckpointVersion        int                `json:"checkpointVersion,omitempty"`
	ExecutionPhase           string             `json:"executionPhase,omitempty"`
	ActionID                 string             `json:"actionId,omitempty"`
	ApprovalRequestID        string             `json:"approvalRequestId,omitempty"`
	Recovery                 *authrail.Recovery `json:"recovery,omitempty"`
	LocalNoDispatchConfirmed bool               `json:"localNoDispatchConfirmed"`
	Next                     string             `json:"next"`
}

// InspectInterruptedRequest runs offline while holding the same lifetime flock
// as the serving connector. It never connects to downstream or Runtime, creates
// credentials, executes work or treats an operator assertion as provider proof.
func InspectInterruptedRequest(settings Settings, toolName, requestKeyValue string, closeBeforeStart bool) (OperatorRequestStatus, error) {
	var empty OperatorRequestStatus
	if !requestKey.MatchString(settings.PrincipalID) || !requestKey.MatchString(settings.SessionID) || !identifier.MatchString(toolName) || !requestKey.MatchString(requestKeyValue) {
		return empty, errors.New("the exact configured tool and saved request key are required")
	}
	var binding *Binding
	for i := range settings.Bindings {
		if settings.Bindings[i].Name == toolName {
			if binding != nil {
				return empty, errors.New("tool binding is ambiguous")
			}
			binding = &settings.Bindings[i]
		}
	}
	if binding == nil {
		return empty, errors.New("the tool is not in the private configuration")
	}
	body, err := readPrivate(filepath.Join(settings.AuthRailDirectory, "connection.json"), 16384)
	if err != nil {
		return empty, err
	}
	var connection struct {
		OrganisationID string `json:"organisationId"`
		DeploymentID   string `json:"deploymentId"`
	}
	if json.Unmarshal(body, &connection) != nil || !uuid.MatchString(connection.OrganisationID) || !uuid.MatchString(connection.DeploymentID) {
		return empty, errors.New("saved installation identity is invalid")
	}
	journalKey, err := readPrivate(filepath.Join(settings.StateDirectory, "journal-key"), 32)
	if err != nil {
		return empty, err
	}
	state, err := openStore(settings.StateDirectory, journalKey)
	if err != nil {
		return empty, err
	}
	defer state.close()
	key := stateKey(connection.OrganisationID + ":" + connection.DeploymentID + ":" + settings.PrincipalID + ":" + toolName + ":" + requestKeyValue)
	saved, err := state.load(key)
	if err != nil {
		return empty, err
	}
	pin, err := hashJSON(*binding)
	if err != nil || saved == nil || saved.Binding != pin || saved.PrincipalID != settings.PrincipalID || saved.SessionID != settings.SessionID || saved.OperationVersionID != binding.OperationVersionID {
		return empty, errors.New("the exact saved request and original binding are unavailable; preserve the original private configuration")
	}
	status := operatorRequestStatus(connection.OrganisationID, connection.DeploymentID, toolName, requestKeyValue, saved)
	if closeBeforeStart {
		if saved.State == "interrupted_before_start" {
			return status, nil
		}
		if saved.State != "checking" || !status.LocalNoDispatchConfirmed {
			return status, errors.New("only a versioned checkpoint proving no SDK start can be closed; preserve uncertain work for trusted investigation")
		}
		// The checkpoint was durably saved before Enforce, and the mandatory SDK
		// callback must advance it before any start request. No authority is renewed.
		saved.State = "interrupted_before_start"
		if err = state.save(key, saved); err != nil {
			return empty, err
		}
		status = operatorRequestStatus(connection.OrganisationID, connection.DeploymentID, toolName, requestKeyValue, saved)
	}
	return status, nil
}

func operatorRequestStatus(organisationID, deploymentID, toolName, key string, saved *record) OperatorRequestStatus {
	beforeStart := saved.CheckpointVersion == recoveryCheckpointVersion && saved.ExecutionPhase == phaseBeforeSDKStart && saved.Recovery == nil && saved.ActionID == "" && saved.ApprovalID == "" && saved.Continuation == nil
	status := OperatorRequestStatus{OrganisationID: organisationID, DeploymentID: deploymentID, PrincipalID: saved.PrincipalID, SessionID: saved.SessionID, Tool: toolName, RequestKey: key,
		Status: saved.State, SavedState: saved.State, CheckpointVersion: saved.CheckpointVersion, ExecutionPhase: saved.ExecutionPhase,
		ActionID: saved.ActionID, ApprovalRequestID: saved.ApprovalID, Recovery: saved.Recovery, LocalNoDispatchConfirmed: beforeStart,
		Next: "Inspect the retained Action and original provider identity. This command does not execute or reconcile work."}
	if saved.State == "checking" || saved.State == "started" || saved.State == "outcome_unknown" {
		if beforeStart {
			status.Status = "interrupted_before_start"
			status.Next = "No SDK execution start or downstream dispatch occurred. Use --close-interrupted to retain this local closure; any new work needs separate authorization."
		} else {
			status.Status = "outcome_unknown"
			status.Next = "Effects remain unknown. Keep the original journal and request key. Use trusted application reconciliation against the original provider identity; do not resubmit or reset state."
		}
	}
	if saved.State == "interrupted_before_start" {
		status.Next = "Closed locally before SDK execution start. No provider outcome was asserted. This request will not execute again; any new work needs separate authorization."
	}
	return status
}
