package protectedmcp

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Setup must match exact selected contracts, not an agent-supplied version or an upstream description.
func TestPrepareBindingsPinsOnlyMatchingReviewedOperations(t *testing.T) {
	for _, scenario := range []string{"matching", "different-input", "ambiguous", "observe", "expired"} {
		t.Run(scenario, func(t *testing.T) {
			config, downstream, sidecar := fixture(t)
			directory := t.TempDir()
			encode := func(v any) []byte {
				b, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}
				return b
			}
			write := func(name string, body []byte) {
				if err := os.WriteFile(filepath.Join(directory, name), body, 0600); err != nil {
					t.Fatal(err)
				}
			}
			write("connection.json", encode(map[string]string{"organisationId": config.Runtime.OrganisationID, "deploymentId": config.Runtime.DeploymentID}))
			write("adapter-token", []byte(config.Runtime.AdapterToken))
			key, err := x509.MarshalPKCS8PrivateKey(config.Runtime.ExecutorPrivateKey)
			if err != nil {
				t.Fatal(err)
			}
			write("executor-private.pem", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))
			input, err := ReviewedInputSchema(downstream.tool.InputSchema, "idempotencyKey")
			if err != nil {
				t.Fatal(err)
			}
			reviewedInput := input
			if scenario == "different-input" {
				reviewedInput = map[string]any{"type": "object", "properties": map[string]any{"other": map[string]any{"type": "string"}}}
			}
			operation := map[string]any{"operationVersionId": operationID, "contract": map[string]any{"summary": "Pay the bound invoice once.", "inputSchema": reviewedInput, "resultSchema": OperationResultSchema(downstream.tool), "requiredCoverage": []string{"tool_call", "tool_result"}, "idempotency": "required", "expectedEffects": []any{map[string]any{"kind": "write"}}, "resourceIdentity": map[string]any{"field": []string{"invoiceId"}}}}
			operations := []any{operation}
			if scenario == "ambiguous" {
				duplicate := map[string]any{"operationVersionId": approvalID, "contract": operation["contract"]}
				operations = append(operations, duplicate)
			}
			mode := "enforce"
			if scenario == "observe" {
				mode = "observe"
			}
			expiry := time.Now().Add(time.Hour)
			if scenario == "expired" {
				expiry = time.Now().Add(-time.Second)
			}
			sidecar.setup = map[string]any{"formatVersion": 1, "organisationId": config.Runtime.OrganisationID, "deploymentId": config.Runtime.DeploymentID, "bundleId": permitID, "sequence": 1, "mode": mode, "expiresAt": expiry, "operations": operations}
			binding := config.Bindings[0]
			binding.OperationVersionID = ""
			binding.SchemaSHA256 = ""
			settings := Settings{AuthRailDirectory: directory, SidecarURL: config.Runtime.SidecarURL, PrincipalID: config.PrincipalID, SessionID: config.SessionID, StateDirectory: config.StateDirectory, Bindings: []Binding{binding}}
			prepared, err := PrepareBindings(context.Background(), settings, downstream)
			if scenario != "matching" {
				if err == nil {
					t.Fatal("unreviewed, ambiguous or unusable deployment matched")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if downstream.effect != 0 || prepared.Bindings[0].OperationVersionID != operationID || prepared.Bindings[0].SchemaSHA256 != config.Bindings[0].SchemaSHA256 || settings.Bindings[0].OperationVersionID != "" {
				t.Fatal("setup ran a business operation or changed the candidate choices")
			}
			path := filepath.Join(t.TempDir(), "private", "prepared.json")
			if err = SavePreparedSettings(path, prepared); err != nil {
				t.Fatal(err)
			}
			if err = SavePreparedSettings(path, prepared); err == nil {
				t.Fatal("installed configuration overwritten")
			}
			saved, err := LoadSettings(path)
			if err != nil || saved.Bindings[0].OperationVersionID != operationID {
				t.Fatal("prepared private binding was not retained")
			}
		})
	}
}
