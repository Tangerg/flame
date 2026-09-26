package runtimebinding

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
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
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
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
	connection := openIntegrationRuntime(t, t.TempDir())
	if _, err := connection.SetApprovalMode(t.Context(), mode); err != nil {
		t.Fatal(err)
	}
	session, err := connection.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	opened, err := connection.StartRun(ctx, testStartRequest(session.ID, agent.Message{Text: "steer admission probe"}))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstEntered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	receipt, err := connection.SteerRun(ctx, agent.SteerRun{
		CommandID: "cli_77777777777777777777777777777777",
		RunID:     opened.RunID, SegmentID: opened.SegmentID,
		Message: agent.Message{Text: "finish with the steered marker"},
	})
	if err != nil || receipt.UserItemID == "" {
		t.Fatalf("accepted steer receipt = %+v, %v", receipt, err)
	}
	accepted, err := connection.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	findReceiptItem := func(block agent.Block) bool {
		return block.RunID == opened.RunID && block.ID == receipt.UserItemID && block.Kind == agent.BlockUser
	}
	if slices.ContainsFunc(accepted.Transcript, findReceiptItem) {
		t.Fatal("steer acceptance already claimed application before the model boundary")
	}
	finishFirst.Do(func() { close(releaseFirst) })
	if mode == protocol.ApprovalModeSafe {
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
		if !active || root.Status != protocol.RunStatusWaiting || len(waiting.Interactions) != 1 ||
			slices.ContainsFunc(waiting.Transcript, findReceiptItem) {
			t.Fatalf("accepted steer crossed the model boundary before approval: %+v", waiting)
		}
		previousSegment := opened.SegmentID
		opened, err = connection.ResumeRun(ctx, agent.ResumeRun{RunID: opened.RunID, Answers: []agent.InterruptAnswer{{
			ItemID: agent.InteractionItemID(waiting.Interactions[0]), Answer: agent.ApprovalAnswer{Decision: protocol.ApprovalApprove},
		}}})
		if err != nil || opened.SegmentID == previousSegment {
			t.Fatalf("resume after accepted steer: segment=%s err=%v", opened.SegmentID, err)
		}
	}
	select {
	case <-secondEntered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	applied, err := connection.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(applied.Transcript, func(block agent.Block) bool {
		return findReceiptItem(block) && block.Status == agent.BlockStatusCompleted && block.Text == "finish with the steered marker"
	}) {
		t.Fatalf("applied receipt Item %s is absent from the authoritative transcript", receipt.UserItemID)
	}
	finishSecond.Do(func() { close(releaseSecond) })
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
