package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestA2AOneVersionAndOriginalTaskBoundary(t *testing.T) {
	binding := &A2ABinding{Version: "1.0", MessageID: "original-message", TaskID: "original-task", ContextID: "original-context"}
	for _, example := range []struct {
		method, params string
		valid          bool
	}{
		{"GetTask", `{"id":"original-task","historyLength":0}`, true},
		{"CancelTask", `{"id":"original-task"}`, true},
		{"GetTask", `{"id":"foreign-task","historyLength":0}`, false},
		{"GetTask", `{"id":"original-task","historyLength":1}`, false},
		{"CancelTask", `{"id":"original-task","endpoint":"https://attacker.invalid"}`, false},
		{"message/send", `{}`, false},
		{"SubscribeToTask", `{"id":"original-task"}`, false},
	} {
		t.Run(example.method+example.params, func(t *testing.T) {
			err := validateA2ARequest(Resolution{Method: example.method, Params: example.params, A2A: binding})
			if (err == nil) != example.valid {
				t.Fatalf("valid=%v err=%v", example.valid, err)
			}
		})
	}
	resolution := Resolution{Method: "GetTask", A2A: binding}
	for _, raw := range []string{
		`{"id":"foreign-task","contextId":"original-context","status":{"state":"TASK_STATE_COMPLETED"}}`,
		`{"id":"original-task","contextId":"foreign-context","status":{"state":"TASK_STATE_COMPLETED"}}`,
		`{"id":"original-task","contextId":"original-context","status":{"state":"invented_success"}}`,
		`{"id":"original-task","contextId":"original-context","status":{"state":"TASK_STATE_COMPLETED"},"artifacts":[{"artifactId":"file","parts":[{"url":"https://attacker.invalid/private"}]}]}`,
	} {
		if _, err := normalizeA2AResult(resolution, json.RawMessage(raw)); err == nil {
			t.Fatalf("unsafe response accepted: %s", raw)
		}
	}
}
func TestA2ARetainsPendingTaskAndPollsWithoutAnotherSend(t *testing.T) {
	var sends, gets, cancels, retained, invalidRetained atomic.Int32
	task := `{"id":"task-1","contextId":"context-1","status":{"state":"TASK_STATE_WORKING"}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/a2a/reviewed-target" || r.Header.Get("A2A-Version") != "1.0" || r.Header.Get("Authorization") != "Bearer private-account" {
			t.Error("recipient or protocol credentials changed")
		}
		var request struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Fatal("invalid request")
		}
		result := task
		switch request.Method {
		case "SendMessage":
			sends.Add(1)
			result = `{"task":` + task + `}`
		case "GetTask":
			gets.Add(1)
		case "CancelTask":
			cancels.Add(1)
		default:
			t.Error("unexpected A2A method")
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"authrail","result":` + result + `}`))
	}))
	defer upstream.Close()
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/internal/gateway/request":
			authorization := "Bearer private-account"
			_ = json.NewEncoder(w).Encode(RequestPermission{Allowed: true, AttemptToken: strings.Repeat("a", 32), Authorization: &authorization, DeadlineAt: time.Now().Add(time.Minute)})
		case "/internal/gateway/response":
			var observation ResponseCheck
			if json.NewDecoder(r.Body).Decode(&observation) != nil || observation.A2A == nil || observation.A2A.TaskID != "task-1" || observation.A2A.State != "TASK_STATE_WORKING" || len(observation.A2A.SnapshotSHA256) != 64 {
				t.Error("original task evidence lost")
			}
			if observation.ReviewedContractValid != nil && !*observation.ReviewedContractValid {
				invalidRetained.Add(1)
			}
			retained.Add(1)
			_, _ = w.Write([]byte(`{"retained":true}`))
		default:
			t.Error("unexpected private API route")
			w.WriteHeader(404)
		}
	}))
	defer apiServer.Close()
	api, err := NewAPI(apiServer.URL, strings.Repeat("s", 32), true)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := NewDispatcher(api, &Configuration{key: strings.Repeat("k", 32)}, upstream.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	binding := &A2ABinding{Version: "1.0", MessageID: "original-message"}
	send := Resolution{Route: Route{Target: "reviewed-target", Protocol: "a2a"}, Method: "SendMessage", Params: `{"message":{"messageId":"original-message","role":"ROLE_USER","parts":[{"data":{"exact":9007199254740993},"mediaType":"application/json"}]},"configuration":{"acceptedOutputModes":["application/json"],"returnImmediately":true,"historyLength":0}}`, A2A: binding}
	send.ReviewedTool = &DiscoveryTool{Name: "reviewed-skill", InputSchema: json.RawMessage(`{"type":"object","required":["exact"],"properties":{"exact":{"type":"integer"}},"additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","required":["task"]}`)}
	if _, err = dispatcher.callA2A(context.Background(), strings.Repeat("t", 32), send); err != nil {
		t.Fatal(err)
	}
	bound := &A2ABinding{Version: "1.0", MessageID: "original-message", TaskID: "task-1", ContextID: "context-1"}
	for _, method := range []string{"GetTask", "GetTask", "CancelTask"} {
		params := `{"id":"task-1"}`
		if method == "GetTask" {
			params = `{"id":"task-1","historyLength":0}`
		}
		resolution := Resolution{Route: send.Route, Method: method, Params: params, A2A: bound, ReviewedTool: send.ReviewedTool}
		if _, err = dispatcher.callA2A(context.Background(), strings.Repeat("t", 32), resolution); err != nil {
			t.Fatal(err)
		}
	}
	if sends.Load() != 1 || gets.Load() != 2 || cancels.Load() != 1 || retained.Load() != 4 {
		t.Fatal("reconnect or cancel replayed business message")
	}
	send.ReviewedTool.OutputSchema = json.RawMessage(`{"type":"object","required":["message"]}`)
	invalid := Resolution{Route: send.Route, Method: "GetTask", Params: `{"id":"task-1","historyLength":0}`, A2A: bound, ReviewedTool: send.ReviewedTool}
	if _, err = dispatcher.callA2A(context.Background(), strings.Repeat("t", 32), invalid); err == nil {
		t.Fatal("unreviewed result was accepted")
	}
	if invalidRetained.Load() != 1 || retained.Load() != 5 || sends.Load() != 1 {
		t.Fatal("invalid result lost original task evidence or repeated message")
	}

}
