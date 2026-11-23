package runtimebinding

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	runworkflow "github.com/Tangerg/flame/apps/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	coldTailRootInput  = "Prove a snapshot tail with overlapping child runs."
	coldTailPreview    = "Write the split preview."
	coldTailSpawner    = "Start a nested run after attachment."
	coldTailNested     = "Finish the nested run."
	coldTailFirstText  = "preview before attachment "
	coldTailSecondText = "preview after nested start"
	coldTailPreviewID  = "preview"
	coldTailSpawnerID  = "spawn"
	coldTailNestedID   = "nested"
	coldTailModel      = "deepseek-chat"
)

func TestRecoveredChildPreviewSurvivesAnUnrelatedChildStart(t *testing.T) {
	configureIntegrationRuntime(t)
	fixtureCtx, stop := context.WithCancel(t.Context())
	defer stop()
	allowNested, allowPreviewTail := make(chan struct{}), make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if err := serveOverlappingChildren(fixtureCtx, w, request, allowNested, allowPreviewTail); err != nil && fixtureCtx.Err() == nil {
			t.Error(err)
		}
	}))
	t.Cleanup(provider.Close)
	t.Setenv("FLAME_PROVIDER", "deepseek")
	t.Setenv("FLAME_MODEL", coldTailModel)
	t.Setenv("FLAME_BASEURL", provider.URL)
	workspace := runtimefixture.TempDirectory(t)
	connection := openIntegrationRuntime(t, runtimefixture.TempDirectory(t))
	session, err := connection.CreateSession(fixtureCtx, conversation.CreateSession{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(fixtureCtx, 45*time.Second)
	defer cancel()
	opened, err := connection.StartRun(ctx, authoring.StartRun{
		SessionID: session.ID, Message: authoring.Message{Text: coldTailRootInput},
		Options: authoring.RunOptions{Provider: "deepseek", Model: coldTailModel},
	})
	if err != nil {
		t.Fatalf("open root run: %v (context cause: %v)", err, context.Cause(ctx))
	}
	var previewRun, previewBlock string
	for event, err := range opened.Events {
		if err != nil {
			t.Fatal(err)
		}
		if delta, ok := event.Event.(conversation.BlockDelta); ok && delta.Text == coldTailFirstText {
			previewRun, previewBlock = event.RunID, delta.BlockID
			break
		}
	}
	if previewRun == "" || previewRun == opened.RunID {
		t.Fatalf("fixture did not open a transient child preview: %q / %q", previewRun, previewBlock)
	}
	recovered, err := runworkflow.RecoverSegment(ctx, connection, session.ID, opened.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Run.Status != protocol.RunStatusRunning || slices.ContainsFunc(recovered.Snapshot.Transcript, func(block conversation.Block) bool {
		return block.ID == previewBlock
	}) {
		t.Fatalf("the snapshot did not cut through the transient preview: %+v", recovered.Snapshot)
	}
	projection := conversation.New()
	if err := projection.RestoreAttachedSnapshot(recovered.Snapshot, recovered.Stream); err != nil {
		t.Fatal(err)
	}
	close(allowNested)
	nestedStarted, previewContinued := false, false
	for event, err := range recovered.Stream.Events {
		if err != nil {
			t.Fatal(err)
		}
		if _, err := projection.ApplyRunEvent(event); err != nil {
			t.Fatalf("nestedStarted=%t, apply observed %T from %s/%s: %v", nestedStarted, event.Event, event.RunID, event.SegmentID, err)
		}
		if started, ok := event.Event.(conversation.SegmentStarted); ok &&
			!started.Run.Lineage.IsRoot() && started.Run.Lineage.ParentRunID() != opened.RunID && !nestedStarted {
			nestedStarted = true
			close(allowPreviewTail)
		}
		if delta, ok := event.Event.(conversation.BlockDelta); ok && event.RunID == previewRun && delta.BlockID == previewBlock && delta.Text == coldTailSecondText {
			previewContinued = true
		}
	}
	if !nestedStarted || !previewContinued || projection.Outcome().Status != protocol.OutcomeCompleted {
		t.Fatalf("nestedStarted=%t, previewContinued=%t, root=%+v", nestedStarted, previewContinued, projection.Outcome())
	}
	if !slices.ContainsFunc(projection.Blocks(), func(block conversation.Block) bool {
		return block.RunID == previewRun && block.ID == previewBlock && block.Text == coldTailFirstText+coldTailSecondText
	}) {
		t.Fatal("original completed child preview is missing")
	}
}

func serveOverlappingChildren(ctx context.Context, w http.ResponseWriter, request *http.Request, allowNested, allowPreviewTail <-chan struct{}) error {
	var in struct {
		Stream   bool `json:"stream"`
		Messages []struct {
			Role       string         `json:"role"`
			Content    jsontext.Value `json:"content"`
			ToolCallID string         `json:"tool_call_id"`
		} `json:"messages"`
	}
	if err := json.UnmarshalRead(request.Body, &in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return err
	}
	for _, message := range slices.Backward(in.Messages) {
		switch {
		case message.Role == "tool" && (message.ToolCallID == coldTailPreviewID || message.ToolCallID == coldTailSpawnerID || message.ToolCallID == coldTailNestedID):
			return writeColdTailCompletion(w, in.Stream, "done", nil)
		case message.Role != "user":
			continue
		case strings.Contains(string(message.Content), coldTailPreview):
			if !in.Stream {
				http.Error(w, "expected a streaming preview", http.StatusBadRequest)
				return errors.New("the preview provider was called without streaming")
			}
			if err := writeColdTailChunk(w, true, coldTailDelta{Role: "assistant", Content: coldTailFirstText}, nil); err != nil {
				return err
			}
			if err := http.NewResponseController(w).Flush(); err != nil {
				return err
			}
			select {
			case <-allowPreviewTail:
			case <-request.Context().Done():
				return nil
			case <-ctx.Done():
				return nil
			}
			finish := "stop"
			if err := writeColdTailChunk(w, true, coldTailDelta{Content: coldTailSecondText}, &finish); err != nil {
				return err
			}
			_, err := fmt.Fprint(w, "data: [DONE]\n\n")
			return err
		case strings.Contains(string(message.Content), coldTailSpawner):
			select {
			case <-allowNested:
			case <-request.Context().Done():
				return nil
			case <-ctx.Done():
				return nil
			}
			return writeColdTailCompletion(w, in.Stream, "", []coldTailToolCall{delegatedColdTailCall(0, coldTailNestedID, coldTailNested)})
		case strings.Contains(string(message.Content), coldTailNested):
			return writeColdTailCompletion(w, in.Stream, "nested completed", nil)
		case strings.Contains(string(message.Content), coldTailRootInput):
			return writeColdTailCompletion(w, in.Stream, "", []coldTailToolCall{
				delegatedColdTailCall(0, coldTailPreviewID, coldTailPreview), delegatedColdTailCall(1, coldTailSpawnerID, coldTailSpawner),
			})
		}
	}
	return writeColdTailCompletion(w, in.Stream, "ok", nil)
}

type coldTailToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func delegatedColdTailCall(index int, id, instruction string) coldTailToolCall {
	call := coldTailToolCall{Index: index, ID: id, Type: "function"}
	call.Function.Name = "delegate_task"
	call.Function.Arguments = fmt.Sprintf(`{"summary":%q,"instructions":%q}`, id, instruction)
	return call
}

type coldTailDelta struct {
	Role      string             `json:"role,omitempty"`
	Content   string             `json:"content,omitempty"`
	ToolCalls []coldTailToolCall `json:"tool_calls,omitempty"`
}

func writeColdTailCompletion(w http.ResponseWriter, stream bool, content string, calls []coldTailToolCall) error {
	finish := "stop"
	if len(calls) != 0 {
		finish = "tool_calls"
	}
	if err := writeColdTailChunk(w, stream, coldTailDelta{Role: "assistant", Content: content, ToolCalls: calls}, &finish); err != nil {
		return err
	}
	if stream {
		_, err := fmt.Fprint(w, "data: [DONE]\n\n")
		return err
	}
	return nil
}

func writeColdTailChunk(w http.ResponseWriter, stream bool, delta coldTailDelta, finish *string) error {
	type choice struct {
		Index        int            `json:"index"`
		Delta        *coldTailDelta `json:"delta,omitempty"`
		Message      *coldTailDelta `json:"message,omitempty"`
		FinishReason *string        `json:"finish_reason"`
	}
	out := struct {
		ID      string         `json:"id"`
		Object  string         `json:"object"`
		Created int            `json:"created"`
		Model   string         `json:"model"`
		Choices []choice       `json:"choices"`
		Usage   jsontext.Value `json:"usage,omitempty"`
	}{ID: "chatcmpl_cold_tail", Created: 1, Model: coldTailModel, Choices: []choice{{FinishReason: finish}}}
	if finish != nil {
		out.Usage = jsontext.Value(`{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}`)
	}
	if !stream {
		out.Object = "chat.completion"
		out.Choices[0].Message = &delta
		w.Header().Set("Content-Type", "application/json")
		return json.MarshalWrite(w, out)
	}
	out.Object = "chat.completion.chunk"
	out.Choices[0].Delta = &delta
	w.Header().Set("Content-Type", "text/event-stream")
	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}
