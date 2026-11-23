package protectedmcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	authrail "github.com/AuthRail/authrail-sdk-go"
)

func interruptedFixture(t *testing.T, saved *record) (Settings, *store) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	settings := Settings{AuthRailDirectory: root, StateDirectory: filepath.Join(root, "state"), PrincipalID: "application", SessionID: "session", Bindings: []Binding{{Name: "pay", OperationVersionID: operationID}}}
	if err := os.Mkdir(settings.StateDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	key := []byte(strings.Repeat("k", 32))
	if err := os.WriteFile(filepath.Join(settings.StateDirectory, "journal-key"), key, 0600); err != nil {
		t.Fatal(err)
	}
	connection := []byte(`{"organisationId":"44444444-4444-4444-8444-444444444444","deploymentId":"55555555-5555-4555-8555-555555555555"}`)
	if err := os.WriteFile(filepath.Join(root, "connection.json"), connection, 0600); err != nil {
		t.Fatal(err)
	}
	state, err := openStore(settings.StateDirectory, key)
	if err != nil {
		t.Fatal(err)
	}
	saved.Binding, _ = hashJSON(settings.Bindings[0])
	saved.Intent = "intent"
	saved.PrincipalID = settings.PrincipalID
	saved.SessionID = settings.SessionID
	saved.OperationVersionID = operationID
	saved.Arguments = `{"private":"never output"}`
	requestID := stateKey("44444444-4444-4444-8444-444444444444:55555555-5555-4555-8555-555555555555:application:pay:original-request")
	if err = state.save(requestID, saved); err != nil {
		t.Fatal(err)
	}
	return settings, state
}

func TestOperatorRecoveryRequiresStoppedOwnerAndRetainsExactClosure(t *testing.T) {
	settings, state := interruptedFixture(t, &record{State: "checking", CheckpointVersion: 1, ExecutionPhase: phaseBeforeSDKStart})
	if _, err := InspectInterruptedRequest(settings, "pay", "original-request", true); err == nil {
		t.Fatal("active connector lock bypassed")
	}
	state.close()
	inspected, err := InspectInterruptedRequest(settings, "pay", "original-request", false)
	if err != nil || !inspected.LocalNoDispatchConfirmed || inspected.SavedState != "checking" {
		t.Fatalf("inspection: %+v %v", inspected, err)
	}
	closed, err := InspectInterruptedRequest(settings, "pay", "original-request", true)
	if err != nil || closed.SavedState != "interrupted_before_start" {
		t.Fatalf("closure: %+v %v", closed, err)
	}
	again, err := InspectInterruptedRequest(settings, "pay", "original-request", true)
	if err != nil || again.Status != closed.Status {
		t.Fatal("closure not idempotent")
	}
	encoded, _ := json.Marshal(closed)
	if strings.Contains(string(encoded), "never output") || strings.Contains(string(encoded), "arguments") {
		t.Fatal("operator output exposed arguments")
	}
	settings.SessionID = "changed-session"
	if _, err = InspectInterruptedRequest(settings, "pay", "original-request", true); err == nil {
		t.Fatal("changed identity accepted")
	}
}

func TestOperatorCannotCloseLegacyOrUncertainExecution(t *testing.T) {
	for _, saved := range []*record{
		{State: "checking"},
		{State: "checking", CheckpointVersion: 1, ExecutionPhase: phaseSDKStartPossible, Recovery: &authrail.Recovery{ActionID: actionID}},
		{State: "started", CheckpointVersion: 1, ExecutionPhase: phaseDispatchPossible, Recovery: &authrail.Recovery{ActionID: actionID}},
		{State: "checking", CheckpointVersion: 2, ExecutionPhase: phaseBeforeSDKStart},
	} {
		t.Run(saved.State+saved.ExecutionPhase, func(t *testing.T) {
			settings, state := interruptedFixture(t, saved)
			state.close()
			before, err := InspectInterruptedRequest(settings, "pay", "original-request", false)
			if err != nil || before.Status != "outcome_unknown" || before.LocalNoDispatchConfirmed {
				t.Fatalf("uncertainty lost: %+v %v", before, err)
			}
			if _, err = InspectInterruptedRequest(settings, "pay", "original-request", true); err == nil {
				t.Fatal("uncertain request closed")
			}
			after, err := InspectInterruptedRequest(settings, "pay", "original-request", false)
			if err != nil || after.SavedState != saved.State {
				t.Fatal("inspection mutated uncertainty")
			}
		})
	}
}

func TestLocalSDKCheckpointAdvancesBeforeStartAndNeverReplaysClosedRequest(t *testing.T) {
	config, downstream, _ := fixture(t)
	connector, err := New(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer connector.Close()
	key := connector.key("pay", "payment-1")
	// The normal path must retain the dispatch boundary; the lost-start fixture
	// separately proves that an unacknowledged start never reaches this phase.
	result, err := connector.invoke(context.Background(), "pay", request())
	if err != nil || result.Status != "completed" {
		t.Fatalf("invoke: %+v %v", result, err)
	}
	retained, err := connector.state.load(key)
	if err != nil || retained.CheckpointVersion != 1 || retained.ExecutionPhase != phaseDispatchPossible {
		t.Fatalf("dispatch marker: %+v %v", retained, err)
	}
	saved := retained
	saved.State = "interrupted_before_start"
	saved.ExecutionPhase = phaseBeforeSDKStart
	saved.Recovery = nil
	saved.ActionID = ""
	if err = connector.state.save(key, saved); err != nil {
		t.Fatal(err)
	}
	again, err := connector.invoke(context.Background(), "pay", request())
	if err != nil || again.Status != "interrupted_before_start" || downstream.effect != 1 {
		t.Fatal("closed request replayed")
	}
}
