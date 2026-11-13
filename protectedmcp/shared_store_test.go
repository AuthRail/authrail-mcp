package protectedmcp

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	authrail "github.com/AuthRail/authrail-sdk-go"
)

func TestSharedStateAuthenticatesCommandsAndNeverRetriesDispatch(t *testing.T) {
	const organisation = "44444444-4444-4444-8444-444444444444"
	const deployment = "55555555-5555-4555-8555-555555555555"
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := sha256.Sum256(public)
	key := stateKey("business-request")
	arguments := `{"invoiceId":"invoice-1"}`
	intent := sha256.Sum256([]byte(arguments))
	value := &record{Arguments: arguments, OperationVersionID: operationID, PrincipalID: "user-1", SessionID: "session-1",
		Binding: strings.Repeat("b", 64), Intent: hex.EncodeToString(intent[:]), State: "checking"}
	var reads, saves atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runtime/deployment/execution-requests" || r.Method != http.MethodPost ||
			r.Header.Get("Authorization") != "Bearer "+strings.Repeat("t", 40) {
			t.Error("request escaped its fixed authenticated endpoint")
			w.WriteHeader(401)
			return
		}
		var envelope struct {
			Command   string `json:"command"`
			KeySHA256 string `json:"keySha256"`
			Proof     struct {
				IssuedAt  int64  `json:"issuedAt"`
				ExpiresAt int64  `json:"expiresAt"`
				Signature string `json:"signature"`
			} `json:"proof"`
		}
		if json.NewDecoder(r.Body).Decode(&envelope) != nil {
			t.Error("invalid command envelope")
			w.WriteHeader(400)
			return
		}
		raw, decodeErr := base64.StdEncoding.DecodeString(envelope.Command)
		digest := sha256.Sum256(raw)
		statement := strings.Join([]string{"authrail.execution.request.v1", organisation, deployment,
			hex.EncodeToString(digest[:]), strconv.FormatInt(envelope.Proof.IssuedAt, 10), strconv.FormatInt(envelope.Proof.ExpiresAt, 10)}, "\n")
		signature, signatureErr := base64.RawURLEncoding.DecodeString(envelope.Proof.Signature)
		if decodeErr != nil || signatureErr != nil || envelope.KeySHA256 != hex.EncodeToString(fingerprint[:]) ||
			envelope.Proof.ExpiresAt-envelope.Proof.IssuedAt != 30 || !ed25519.Verify(public, []byte(statement), signature) {
			t.Error("command proof did not bind the exact bytes and deployment identity")
			w.WriteHeader(401)
			return
		}
		var command struct {
			Kind       string `json:"kind"`
			Key        string `json:"key"`
			OwnerToken string `json:"ownerToken"`
			Fence      int    `json:"fence"`
			Revision   int    `json:"revision"`
			Record     string `json:"record"`
		}
		if json.Unmarshal(raw, &command) != nil || command.Key != key {
			t.Error("command changed request identity")
			w.WriteHeader(400)
			return
		}
		switch command.Kind {
		case "claim":
			if len(command.OwnerToken) != 64 {
				t.Error("missing opaque ownership token")
			}
		case "read":
			reads.Add(1)
		case "save":
			saves.Add(1)
			if command.Fence != 7 || command.Revision != 3 {
				t.Error("save omitted the acquired fence or revision")
			}
			w.WriteHeader(http.StatusConflict)
			return
		case "release":
			if command.Fence != 7 {
				t.Error("release omitted its exact fence")
			}
		default:
			t.Error("unexpected command")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": organisation, "deploymentId": deployment,
			"fence": 7, "revision": 3, "leaseExpiresAt": time.Now().Add(2 * time.Minute), "record": value, "dispatchDeadlineAt": nil})
	}))
	defer server.Close()
	store, err := openSharedStore(authrail.Config{OrganisationID: organisation, DeploymentID: deployment, ExecutorPrivateKey: private},
		SharedStateConfig{APIURL: server.URL, DeploymentToken: strings.Repeat("t", 40)})
	if err != nil {
		t.Fatal(err)
	}
	release := store.acquire(key)
	defer release()
	if _, err = store.load(key); err != nil {
		t.Fatal(err)
	}
	value.State = "started"
	if err = store.save(key, value); err == nil || saves.Load() != 1 || reads.Load() != 1 {
		t.Fatalf("dispatch conflict was retried: %v saves=%d reads=%d", err, saves.Load(), reads.Load())
	}
	value.State = "outcome_unknown"
	if err = store.save(key, value); err == nil || saves.Load() != 1 {
		t.Fatal("failed ownership allowed a later mutation")
	}
}

func TestSharedStateRejectsForeignIdentityAndFailedClaimMutations(t *testing.T) {
	for _, kind := range []string{"foreign response", "claim conflict"} {
		t.Run(kind, func(t *testing.T) {
			config, _, _ := fixture(t)
			var saves atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var envelope struct {
					Command string `json:"command"`
				}
				_ = json.Unmarshal(body, &envelope)
				raw, _ := base64.StdEncoding.DecodeString(envelope.Command)
				var command struct {
					Kind string `json:"kind"`
				}
				_ = json.Unmarshal(raw, &command)
				if command.Kind == "save" {
					saves.Add(1)
				}
				if command.Kind == "claim" && kind == "claim conflict" {
					w.WriteHeader(http.StatusConflict)
					return
				}
				organisation := config.Runtime.OrganisationID
				if kind == "foreign response" {
					organisation = actionID
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"organisationId": organisation, "deploymentId": config.Runtime.DeploymentID,
					"fence": 1, "revision": 0, "leaseExpiresAt": time.Now().Add(time.Minute), "record": nil, "dispatchDeadlineAt": nil})
			}))
			defer server.Close()
			store, err := openSharedStore(config.Runtime, SharedStateConfig{APIURL: server.URL, DeploymentToken: strings.Repeat("t", 40)})
			if err != nil {
				t.Fatal(err)
			}
			release := store.acquire(stateKey("business-request"))
			defer release()
			_, readErr := store.load(stateKey("business-request"))
			if kind == "foreign response" && readErr == nil {
				t.Fatal("foreign tenant response was accepted")
			}
			if err = store.save(stateKey("business-request"), &record{}); err == nil || saves.Load() != 0 {
				t.Fatal("failed claim reached a mutating API request")
			}
		})
	}
}
