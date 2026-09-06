package runtimebinding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	runapplication "github.com/Tangerg/flame/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestOneShotRecoversADelegatedApprovalBeforeResumingTheRoot(t *testing.T) {
	configureIntegrationRuntime(t)
	provider := httptest.NewServer(http.HandlerFunc(delegatedApprovalResponse))
	t.Cleanup(provider.Close)
	t.Setenv("FLAME_PROVIDER", "deepseek")
	t.Setenv("FLAME_MODEL", "deepseek-chat")
	t.Setenv("FLAME_BASEURL", provider.URL)

	connection := openIntegrationRuntime(t, t.TempDir())
	if _, err := connection.SetApprovalMode(t.Context(), protocol.ApprovalModeSafe); err != nil {
		t.Fatal(err)
	}
	session, err := connection.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	profile := connection.Profile()
	replay, err := CommandReplayPolicy(&profile)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &interruptedRootStream{Connection: connection}
	renderer := new(recoveryRenderer)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := runapplication.Execute(ctx, runapplication.Invocation{
		Runtime: runtime, Renderer: renderer, ReplayPolicy: replay, ApproveAll: true, ReconnectAttempts: 2,
		Start: agent.StartRun{
			SessionID: session.ID, Message: agent.Message{Text: "delegate approval probe"},
			Options: agent.RunOptions{Provider: "deepseek", Model: "deepseek-chat", Limits: agent.UnlimitedRunLimits()},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if !runtime.disconnected || runtime.subscriptions == 0 || runtime.resumptions != 1 {
		t.Fatalf("recovery: disconnected=%t subscriptions=%d resumptions=%d", runtime.disconnected, runtime.subscriptions, runtime.resumptions)
	}
	snapshot, err := connection.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	root, ok := snapshot.LatestRun()
	if !ok || root.Status != protocol.RunStatusFinished || root.Outcome.Status != agent.OutcomeCompleted || len(snapshot.Interactions) != 0 {
		t.Fatalf("root after recovery = %+v, interactions = %+v", root, snapshot.Interactions)
	}
	if len(snapshot.Runs) != 2 {
		t.Fatalf("continuation changed the admitted root and child: %+v", snapshot.Runs)
	}
	if !slices.ContainsFunc(snapshot.Transcript, func(block agent.Block) bool {
		return block.RunID != root.ID && block.Tool != nil && block.Tool.Name == "shell" &&
			block.Tool.Status == agent.ToolOK && strings.Contains(block.Tool.Output, "approved")
	}) {
		t.Fatalf("approved child tool did not complete: %+v", snapshot.Transcript)
	}
	if !slices.ContainsFunc(renderer.events, func(event agent.RunEvent) bool {
		_, finished := event.Event.(agent.RunFinished)
		return event.RunID == root.ID && finished
	}) {
		t.Fatal("the recovered root completion was not rendered")
	}
}

// The durable barrier is committed, but this consumer loses the stream between
// a child's interrupt and the root's suspension event.
type interruptedRootStream struct {
	*Connection
	disconnected  bool
	subscriptions int
	resumptions   int
}

func (r *interruptedRootStream) StartRun(ctx context.Context, request agent.StartRun) (agent.SegmentStream, error) {
	stream, err := r.Connection.StartRun(ctx, request)
	if err != nil {
		return agent.SegmentStream{}, err
	}
	events := stream.Events
	stream.Events = func(yield func(agent.RunEvent, error) bool) {
		for event, streamErr := range events {
			if _, suspended := event.Event.(agent.RunSuspended); suspended && event.RunID == stream.RunID {
				r.disconnected = true
				yield(agent.RunEvent{}, agent.ErrDisconnected)
				return
			}
			if !yield(event, streamErr) {
				return
			}
		}
	}
	return stream, nil
}

func (r *interruptedRootStream) SubscribeRun(ctx context.Context, request agent.SubscribeRun) (agent.SegmentStream, error) {
	r.subscriptions++
	return r.Connection.SubscribeRun(ctx, request)
}

func (r *interruptedRootStream) ResumeRun(ctx context.Context, request agent.ResumeRun) (agent.SegmentStream, error) {
	r.resumptions++
	return r.Connection.ResumeRun(ctx, request)
}

type recoveryRenderer struct{ events []agent.RunEvent }

func (*recoveryRenderer) Begin(agent.Run, agent.RunOptions) error { return nil }
func (*recoveryRenderer) Reconcile(agent.SessionSnapshot) error   { return nil }
func (*recoveryRenderer) Close() error                            { return nil }
func (r *recoveryRenderer) Render(event agent.RunEvent) error {
	r.events = append(r.events, event.Clone())
	return nil
}

func delegatedApprovalResponse(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Stream   bool `json:"stream"`
		Messages []struct {
			Role       string          `json:"role"`
			Content    json.RawMessage `json:"content"`
			ToolCallID string          `json:"tool_call_id"`
		} `json:"messages"`
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	content, callID, name, arguments := "maintenance complete", "", "", ""
	if len(request.Tools) > 0 {
		matched := false
		for _, message := range slices.Backward(request.Messages) {
			switch {
			case message.Role == "tool" && message.ToolCallID == "child_shell":
				content = "child complete"
			case message.Role == "tool" && message.ToolCallID == "delegate_child":
				content = "root complete"
			case message.Role == "user" && strings.Contains(string(message.Content), "child approval probe"):
				callID, name, arguments = "child_shell", "shell", `{"command":"printf approved","description":"Return the approved marker"}`
			case message.Role == "user" && strings.Contains(string(message.Content), "delegate approval probe"):
				callID, name, arguments = "delegate_child", "delegate_task", `{"summary":"approval probe","instructions":"child approval probe"}`
			default:
				continue
			}
			matched = true
			break
		}
		if !matched {
			http.Error(w, "unexpected delegated approval request", http.StatusBadRequest)
			return
		}
	}
	delta := map[string]any{"role": "assistant", "content": content}
	finish := "stop"
	if name != "" {
		delta = map[string]any{
			"role": "assistant", "tool_calls": []any{map[string]any{
				"index": 0, "id": callID, "type": "function",
				"function": map[string]any{"name": name, "arguments": arguments},
			}},
		}
		finish = "tool_calls"
	}
	choice := map[string]any{"index": 0, "finish_reason": finish}
	object := "chat.completion"
	if request.Stream {
		object = "chat.completion.chunk"
		choice["delta"] = delta
	} else {
		choice["message"] = delta
	}
	body := map[string]any{
		"id": "chatcmpl_probe", "object": object, "created": 1, "model": "deepseek-chat",
		"choices": []any{choice}, "usage": map[string]int{"prompt_tokens": 4, "completion_tokens": 2, "total_tokens": 6},
	}
	if request.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		data, err := json.Marshal(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}
