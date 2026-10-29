package main

import (
	"context"
	stdjson "encoding/json"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/a2a/internal/contract"
	"github.com/Tangerg/flame/plugins/a2a/internal/wire"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

type remoteFixture struct {
	mu         sync.Mutex
	server     *httptest.Server
	advertised string
	cards      int
	calls      map[string]int
	lost       map[string]bool
	task       a2a.Task
	message    *a2a.Message
	block      string
	entered    chan struct{}
}

func newRemoteFixture(t *testing.T) *remoteFixture {
	t.Helper()
	f := &remoteFixture{calls: make(map[string]int), lost: make(map[string]bool), task: a2a.Task{ID: "remote-task", ContextID: "remote-context", Status: a2a.TaskStatus{State: a2a.TaskStateWorking}}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		if r.URL.Path == "/card" {
			f.cards++
			card := a2a.AgentCard{Name: "remote", SupportedInterfaces: []*a2a.AgentInterface{{URL: f.advertised, ProtocolBinding: a2a.TransportProtocolJSONRPC, ProtocolVersion: a2a.Version}}}
			f.mu.Unlock()
			if err := json.MarshalWrite(w, card); err != nil {
				t.Error(err)
			}
			return
		}
		f.mu.Unlock()
		if r.URL.Path != "/rpc" {
			t.Errorf("request moved to %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var request struct {
			ID     stdjson.RawMessage `json:"id"`
			Method string             `json:"method"`
			Params stdjson.RawMessage `json:"params"`
		}
		if err := json.UnmarshalRead(r.Body, &request); err != nil {
			t.Error(err)
			return
		}
		f.mu.Lock()
		f.calls[request.Method]++
		lost, task, message, block, entered := f.lost[request.Method], f.task, f.message, f.block == request.Method, f.entered
		f.mu.Unlock()
		if block {
			close(entered)
			<-r.Context().Done()
			return
		}
		if lost {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0"`))
			return
		}
		var result any = task
		switch request.Method {
		case "SendMessage":
			var input a2a.SendMessageRequest
			if err := json.Unmarshal(request.Params, &input); err != nil {
				t.Error(err)
				return
			}
			if input.Config == nil || !input.Config.ReturnImmediately || input.Message == nil || input.Message.ID == "" {
				t.Error("send must request immediate response and carry a message identity")
			}
			if message != nil {
				result = struct {
					Message *a2a.Message `json:"message"`
				}{message}
			} else {
				result = struct {
					Task a2a.Task `json:"task"`
				}{task}
			}
		case "GetTask", "CancelTask":
			var input struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(request.Params, &input); err != nil || input.ID != "remote-task" {
				t.Errorf("lost original task identity: %s %v", request.Params, err)
			}
		default:
			t.Errorf("unexpected RPC %s", request.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, struct {
			JSONRPC string             `json:"jsonrpc"`
			ID      stdjson.RawMessage `json:"id"`
			Result  any                `json:"result"`
		}{"2.0", request.ID, result}); err != nil {
			t.Error(err)
		}
	}))
	f.advertised = f.server.URL + "/rpc"
	t.Cleanup(f.server.Close)
	return f
}

func (f *remoteFixture) spec() agentSpec {
	return agentSpec{Name: "research", CardURL: f.server.URL + "/card"}
}

func newTestConnector(t *testing.T, path string, spec agentSpec) *connector {
	t.Helper()
	c, err := openConnector(t.Context(), path, []agentSpec{spec})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.db.Close() })
	return c
}

func inputValue(t *testing.T, value any) jsonvalue.Value {
	t.Helper()
	encoded, err := jsonvalue.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func testContracts(t *testing.T) rpc.Contracts {
	t.Helper()
	c, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTaskBindingSurvivesRestartAndCardReplacement(t *testing.T) {
	remote := newRemoteFixture(t)
	path := filepath.Join(t.TempDir(), "a2a.db")
	c := newTestConnector(t, path, remote.spec())
	contracts := testContracts(t)
	input := inputValue(t, wire.SendRequest{Agent: "research", Message: "inspect the project"})
	original, err := c.send(t.Context(), contracts.Send.Name(), "send-original", input)
	if err != nil || original.TaskID != "remote-task" || original.ContextID != "remote-context" || original.State != string(a2a.TaskStateWorking) {
		t.Fatalf("original send: %+v %v", original, err)
	}
	if err := c.db.Close(); err != nil {
		t.Fatal(err)
	}
	remote.mu.Lock()
	remote.advertised = remote.server.URL + "/replacement"
	remote.task.Status.State = a2a.TaskStateCompleted
	remote.mu.Unlock()
	c = newTestConnector(t, path, remote.spec())
	status, err := c.status(t.Context(), wire.TaskRequest{RequestID: original.RequestID})
	if err != nil || status.State != string(a2a.TaskStateCompleted) {
		t.Fatalf("status: %+v %v", status, err)
	}
	replayed, err := c.send(t.Context(), contracts.Send.Name(), "send-original", input)
	if err != nil || replayed != original {
		t.Fatalf("original receipt changed: %+v %v", replayed, err)
	}
	receipt, err := pluginstorage.ReadReceipt(t.Context(), c.db, qimumcp.ReceiptRequest{ID: "send-original", Method: contracts.Send.Name(), InputDigest: input.Digest()})
	if err != nil || !receipt.Found {
		t.Fatalf("durable receipt: %+v %v", receipt, err)
	}
	decoded, err := qimumcp.DecodeReply[wire.Result](receipt.Reply)
	if err != nil || decoded != original {
		t.Fatalf("receipt: %+v %v", decoded, err)
	}
	remote.mu.Lock()
	defer remote.mu.Unlock()
	if remote.cards != 1 || remote.calls["SendMessage"] != 1 || remote.calls["GetTask"] != 1 {
		t.Fatalf("unexpected network calls: cards=%d rpc=%v", remote.cards, remote.calls)
	}
}

func TestLostMutationRepliesNeverResend(t *testing.T) {
	for _, method := range []string{"SendMessage", "CancelTask"} {
		t.Run(method, func(t *testing.T) {
			remote := newRemoteFixture(t)
			path := filepath.Join(t.TempDir(), "a2a.db")
			c := newTestConnector(t, path, remote.spec())
			contracts := testContracts(t)
			input := inputValue(t, wire.SendRequest{Agent: "research", Message: "work"})
			if method == "CancelTask" {
				if _, err := c.send(t.Context(), contracts.Send.Name(), "original", input); err != nil {
					t.Fatal(err)
				}
				input = inputValue(t, wire.TaskRequest{RequestID: "original"})
			}
			remote.mu.Lock()
			remote.lost[method] = true
			remote.mu.Unlock()
			invoke := func(c *connector) (wire.Result, error) {
				if method == "SendMessage" {
					return c.send(t.Context(), contracts.Send.Name(), "lost", input)
				}
				return c.cancel(t.Context(), contracts.Cancel.Name(), "lost", input)
			}
			if result, err := invoke(c); err == nil || result.State != "" {
				t.Fatalf("lost result claimed a state: %+v %v", result, err)
			}
			if err := c.db.Close(); err != nil {
				t.Fatal(err)
			}
			c = newTestConnector(t, path, remote.spec())
			if _, err := invoke(c); !errors.Is(err, pluginstorage.ErrCommandUnconfirmed) {
				t.Fatalf("pending retry: %v", err)
			}
			remote.mu.Lock()
			count := remote.calls[method]
			remote.mu.Unlock()
			if count != 1 {
				t.Fatalf("mutation was replayed %d times", count)
			}
		})
	}
}

func TestCancellationProjectsOnlyRemoteState(t *testing.T) {
	remote := newRemoteFixture(t)
	c := newTestConnector(t, filepath.Join(t.TempDir(), "a2a.db"), remote.spec())
	contracts := testContracts(t)
	if _, err := c.send(t.Context(), contracts.Send.Name(), "original", inputValue(t, wire.SendRequest{Agent: "research", Message: "work"})); err != nil {
		t.Fatal(err)
	}
	input := inputValue(t, wire.TaskRequest{RequestID: "original"})
	result, err := c.cancel(t.Context(), contracts.Cancel.Name(), "cancel", input)
	if err != nil || result.State != string(a2a.TaskStateWorking) {
		t.Fatalf("cancel invented a transition: %+v %v", result, err)
	}
	remote.mu.Lock()
	remote.task.Status.State = a2a.TaskStateCanceled
	remote.mu.Unlock()
	status, err := c.status(t.Context(), wire.TaskRequest{RequestID: "original"})
	if err != nil || status.State != string(a2a.TaskStateCanceled) {
		t.Fatalf("remote cancellation: %+v %v", status, err)
	}
	replayed, err := c.cancel(t.Context(), contracts.Cancel.Name(), "cancel", input)
	if err != nil || replayed != result {
		t.Fatalf("original cancellation reply changed: %+v %v", replayed, err)
	}
}

func TestLocalInterruptionRetainsUnconfirmedSend(t *testing.T) {
	remote := newRemoteFixture(t)
	remote.block, remote.entered = "SendMessage", make(chan struct{})
	c := newTestConnector(t, filepath.Join(t.TempDir(), "a2a.db"), remote.spec())
	contracts := testContracts(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	input := inputValue(t, wire.SendRequest{Agent: "research", Message: "work"})
	done := make(chan error, 1)
	go func() { _, err := c.send(ctx, contracts.Send.Name(), "interrupted", input); done <- err }()
	<-remote.entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("interruption: %v", err)
	}
	if _, err := c.send(t.Context(), contracts.Send.Name(), "interrupted", input); !errors.Is(err, pluginstorage.ErrCommandUnconfirmed) {
		t.Fatalf("interruption replay: %v", err)
	}
	if _, err := c.status(t.Context(), wire.TaskRequest{RequestID: "interrupted"}); err == nil {
		t.Fatal("unconfirmed send invented a task binding")
	}
}

func TestChangedAgentAndWrongRemoteIdentityAreRefused(t *testing.T) {
	remote := newRemoteFixture(t)
	path := filepath.Join(t.TempDir(), "a2a.db")
	c := newTestConnector(t, path, remote.spec())
	contracts := testContracts(t)
	if _, err := c.send(t.Context(), contracts.Send.Name(), "original", inputValue(t, wire.SendRequest{Agent: "research", Message: "work"})); err != nil {
		t.Fatal(err)
	}
	remote.mu.Lock()
	remote.task.ContextID = "different-context"
	remote.mu.Unlock()
	if _, err := c.status(t.Context(), wire.TaskRequest{RequestID: "original"}); err == nil {
		t.Fatal("accepted another task context")
	}
	if _, err := c.cancel(t.Context(), contracts.Cancel.Name(), "wrong-task", inputValue(t, wire.TaskRequest{RequestID: "original"})); err == nil {
		t.Fatal("accepted cancellation of another task context")
	}
	if err := c.db.Close(); err != nil {
		t.Fatal(err)
	}
	spec := remote.spec()
	spec.CardURL += "?replacement=true"
	c = newTestConnector(t, path, spec)
	_, err := c.status(t.Context(), wire.TaskRequest{RequestID: "original"})
	refusal, known := errors.AsType[*portable.Refusal](err)
	if !known || refusal.Code != "agent_changed" {
		t.Fatalf("changed source: %v", err)
	}
}

func TestMessageReplyHasNoSyntheticTaskState(t *testing.T) {
	remote := newRemoteFixture(t)
	remote.message = a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("answer"))
	c := newTestConnector(t, filepath.Join(t.TempDir(), "a2a.db"), remote.spec())
	contracts := testContracts(t)
	result, err := c.send(t.Context(), contracts.Send.Name(), "message", inputValue(t, wire.SendRequest{Agent: "research", Message: "question"}))
	if err != nil || result.Kind != "message" || result.TaskID != "" || result.State != "" || !strings.Contains(result.ValueJSON, "answer") {
		t.Fatalf("message: %+v %v", result, err)
	}
	if _, err := c.status(t.Context(), wire.TaskRequest{RequestID: "message"}); err == nil {
		t.Fatal("message reply invented a task")
	}
}
