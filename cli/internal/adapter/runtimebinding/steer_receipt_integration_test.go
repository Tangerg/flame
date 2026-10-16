package runtimebinding

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestSteerReceiptNamesTheUserItemCommittedAtTheNextModelBoundary(t *testing.T) {
	for _, mode := range []protocol.ApprovalMode{protocol.ApprovalModeYolo, protocol.ApprovalModeSafe} {
		t.Run(string(mode), func(t *testing.T) { testSteerReceiptAtModelBoundary(t, mode) })
	}
}

func testSteerReceiptAtModelBoundary(t *testing.T, mode protocol.ApprovalMode) {
	configureIntegrationRuntime(t)
	firstEntered, secondEntered := make(chan struct{}), make(chan struct{})
	releaseFirst, releaseSecond := make(chan struct{}), make(chan struct{})
	var enterFirst, enterSecond, finishFirst, finishSecond sync.Once
	var requestsMu sync.Mutex
	var requests []string
	providerStarted := time.Now()
	release := func() {
		finishFirst.Do(func() { close(releaseFirst) })
		finishSecond.Do(func() { close(releaseSecond) })
	}
	defer release()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var body struct {
			Stream   bool `json:"stream"`
			Messages []struct {
				Role       string         `json:"role"`
				Content    jsontext.Value `json:"content"`
				ToolCallID string         `json:"tool_call_id"`
			} `json:"messages"`
			Tools []jsontext.Value `json:"tools"`
		}
		if err := json.UnmarshalRead(request.Body, &body); err != nil {
			requestsMu.Lock()
			requests = append(requests, fmt.Sprintf("elapsed=%s decode_error=%v", time.Since(providerStarted), err))
			requestsMu.Unlock()
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var toolResults []string
		for _, message := range body.Messages {
			if message.Role == "tool" {
				toolResults = append(toolResults, message.ToolCallID)
			}
		}
		requestsMu.Lock()
		requests = append(requests, fmt.Sprintf("elapsed=%s stream=%t tools=%d tool_result_ids=%q",
			time.Since(providerStarted), body.Stream, len(body.Tools), toolResults))
		requestsMu.Unlock()
		toolCall := false
		if len(body.Tools) != 0 {
			second := false
			for _, message := range body.Messages {
				if message.Role == "tool" && message.ToolCallID == "steer_boundary" {
					second = true
					break
				}
			}
			wait := releaseFirst
			if second {
				enterSecond.Do(func() { close(secondEntered) })
				wait = releaseSecond
			} else {
				enterFirst.Do(func() { close(firstEntered) })
				toolCall = true
			}
			select {
			case <-wait:
			case <-request.Context().Done():
				return
			}
		}
		writeSteerBoundaryResponse(w, body.Stream, toolCall)
	}))
	t.Cleanup(provider.Close)
	t.Setenv("FLAME_PROVIDER", "deepseek")
	t.Setenv("FLAME_MODEL", "deepseek-chat")
	t.Setenv("FLAME_BASEURL", provider.URL)
	connection := openIntegrationRuntime(t, runtimefixture.TempDirectory(t))
	if _, err := connection.SetApprovalMode(t.Context(), mode); err != nil {
		t.Fatal(err)
	}
	session, err := connection.CreateSession(t.Context(), conversation.CreateSession{Workspace: runtimefixture.TempDirectory(t)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	startedAt := time.Now()
	type stageObservation struct {
		name    string
		elapsed time.Duration
	}
	var stages []stageObservation
	mark := func(name string) {
		stages = append(stages, stageObservation{name, time.Since(startedAt)})
	}
	defer func() {
		if !t.Failed() || ctx.Err() != context.DeadlineExceeded {
			return
		}
		for _, stage := range stages {
			t.Logf("steer deadline: mode=%s stage=%s elapsed=%s", mode, stage.name, stage.elapsed)
		}
		for name, entered := range map[string]<-chan struct{}{"first": firstEntered, "second": secondEntered} {
			select {
			case <-entered:
				t.Logf("steer deadline: %s provider entered", name)
			default:
				t.Logf("steer deadline: %s provider has not entered", name)
			}
		}
		// Provider arrival and this fixture's classification are different facts:
		// a request with missing Tools or changed result IDs must remain visible.
		requestsMu.Lock()
		requestSnapshot := slices.Clone(requests)
		requestsMu.Unlock()
		for index, request := range requestSnapshot {
			t.Logf("steer deadline provider request: index=%d %s", index, request)
		}
		stack := make([]byte, 1<<20)
		t.Logf("steer deadline goroutines:\n%s", stack[:runtime.Stack(stack, true)])
		// Inspect after the original deadline fails; diagnostics never extend the
		// success window or consume the stream that the model-boundary assertion tests.
		inspection, stop := context.WithTimeout(context.WithoutCancel(t.Context()), 2*time.Second)
		defer stop()
		observed, err := connection.GetSession(inspection, session.ID)
		t.Logf("steer deadline session: snapshot=%+v error=%v", observed, err)
		for _, run := range observed.Runs {
			t.Logf("steer deadline run: id=%s segment=%s status=%s outcome=%s explanation=%s",
				run.ID, run.ActiveSegmentID, run.Status, run.Outcome.Status, run.Outcome.Explanation())
		}
	}()
	mark("start requested")
	opened, err := connection.StartRun(ctx, testStartRequest(session.ID, prompt.Message{Text: "steer admission probe"}))
	if err != nil {
		t.Fatal(err)
	}
	mark("start acknowledged; waiting for first provider")
	select {
	case <-firstEntered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	mark("first provider entered; steer requested")
	receipt, err := connection.SteerRun(ctx, prompt.SteerRun{
		CommandID: "cli_77777777777777777777777777777777",
		RunID:     opened.RunID, SegmentID: opened.SegmentID,
		Message: prompt.Message{Text: "finish with the steered marker"},
	})
	if err != nil || receipt.UserItemID == "" {
		t.Fatalf("accepted steer receipt = %+v, %v", receipt, err)
	}
	mark("steer acknowledged")
	accepted, err := connection.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	findReceiptItem := func(block conversation.Block) bool {
		return block.RunID == opened.RunID && block.ID == receipt.UserItemID && block.Kind == conversation.BlockUser
	}
	if slices.ContainsFunc(accepted.Transcript, findReceiptItem) {
		t.Fatal("steer acceptance already claimed application before the model boundary")
	}
	finishFirst.Do(func() { close(releaseFirst) })
	if mode == protocol.ApprovalModeSafe {
		mark("first provider released; waiting for approval stream")
		for _, err := range opened.Events {
			if err != nil {
				t.Fatal(err)
			}
		}
		waiting, err := connection.GetSession(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		root, active := waiting.ActiveRun()
		if !active || root.Status != protocol.RunStatusWaiting || len(waiting.Interrupts) != 1 ||
			slices.ContainsFunc(waiting.Transcript, findReceiptItem) {
			t.Fatalf("accepted steer crossed the model boundary before approval: %+v", waiting)
		}
		previousSegment := opened.SegmentID
		mark("waiting snapshot validated; resume requested")
		opened, err = connection.ResumeRun(ctx, conversation.ResumeRun{RunID: opened.RunID, Answers: []conversation.InterruptAnswer{{
			ItemID: conversation.InterruptItemID(waiting.Interrupts[0]), Answer: conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove},
		}}})
		if err != nil || opened.SegmentID == previousSegment {
			t.Fatalf("resume after accepted steer: segment=%s err=%v", opened.SegmentID, err)
		}
		mark("resume acknowledged")
	}
	mark("waiting for second provider")
	select {
	case <-secondEntered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	mark("second provider entered")
	applied, err := connection.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(applied.Transcript, func(block conversation.Block) bool {
		return findReceiptItem(block) && block.Status == conversation.BlockStatusCompleted && block.Text == "finish with the steered marker"
	}) {
		t.Fatalf("applied receipt Item %s is absent from the authoritative transcript", receipt.UserItemID)
	}
	finishSecond.Do(func() { close(releaseSecond) })
	mark("second provider released; waiting for final stream")
	for _, err := range opened.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	finished, err := connection.GetSession(ctx, session.ID)
	if err != nil || !slices.ContainsFunc(finished.Transcript, findReceiptItem) {
		t.Fatalf("finished Run lost its applied steer Item: %v", err)
	}
}

func writeSteerBoundaryResponse(w http.ResponseWriter, streaming, toolCall bool) {
	delta := map[string]any{"role": "assistant", "content": "steer boundary complete"}
	finish := "stop"
	if toolCall {
		delta = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{
			"index": 0, "id": "steer_boundary", "type": "function",
			"function": map[string]any{"name": "shell", "arguments": `{"command":"printf continued","description":"Continue to the next model boundary"}`},
		}}}
		finish = "tool_calls"
	}
	choice := map[string]any{"index": 0, "finish_reason": finish}
	object := "chat.completion"
	if streaming {
		object = "chat.completion.chunk"
		choice["delta"] = delta
	} else {
		choice["message"] = delta
	}
	body := map[string]any{
		"id": "chatcmpl_steer", "object": object, "created": 1, "model": "deepseek-chat",
		"choices": []any{choice}, "usage": map[string]int{"prompt_tokens": 4, "completion_tokens": 2, "total_tokens": 6},
	}
	if streaming {
		w.Header().Set("Content-Type", "text/event-stream")
		encoded, err := json.Marshal(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.MarshalWrite(w, body)
}
