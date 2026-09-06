package bootstrap

import (
	"context"
	"encoding/json"
	"iter"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

func TestProtocolResumesWaitingTreeBesideCompletedSiblingAfterRestart(t *testing.T) {
	waiting := chat.ToolCall{ID: "delegate_a", Name: "delegate_task", Arguments: `{"summary":"A","instructions":"waiting sibling A"}`}
	completed := chat.ToolCall{ID: "delegate_b", Name: "delegate_task", Arguments: `{"summary":"B","instructions":"completed sibling B"}`}
	for _, test := range []struct {
		name  string
		calls []chat.ToolCall
	}{
		{name: "completed successor", calls: []chat.ToolCall{waiting, completed}},
		{name: "completed predecessor", calls: []chat.ToolCall{completed, waiting}},
	} {
		t.Run(test.name, func(t *testing.T) { testProtocolSiblingRestart(t, test.calls) })
	}
}

func testProtocolSiblingRestart(t *testing.T, delegateCalls []chat.ToolCall) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	releaseA := make(chan struct{})
	var calls atomic.Int32
	model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		calls.Add(1)
		var hasResult, waitingChild, completedChild bool
		for _, message := range request.Messages {
			hasResult = hasResult || message.Role == chat.RoleTool
			if message.Role == chat.RoleUser {
				waitingChild = waitingChild || strings.Contains(message.Text(), "waiting sibling A")
				completedChild = completedChild || strings.Contains(message.Text(), "completed sibling B")
			}
		}
		message := chat.NewAssistantMessage(chat.NewTextPart("continued"))
		finish := chat.FinishReasonStop
		switch {
		case hasResult:
		case waitingChild:
			select {
			case <-releaseA:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			message = chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
				ID: "ask_a", Name: "ask_user", Arguments: `{"questions":[{"question":"Continue sibling A?"}]}`,
			}))
			finish = chat.FinishReasonToolCalls
		case completedChild:
			message = chat.NewAssistantMessage(chat.NewTextPart("sibling B completed"))
		default:
			message = chat.NewAssistantMessage(
				chat.NewToolCallPart(delegateCalls[0]),
				chat.NewToolCallPart(delegateCalls[1]),
			)
			finish = chat.FinishReasonToolCalls
		}
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: finish}, &chat.ResponseMetadata{
			Model: "claude-test", Usage: chat.Usage{InputTokens: 2, OutputTokens: 1},
		})
	})}
	ctx := delivery.WithRequestMeta(t.Context(), protocol.RequestMeta{
		ProtocolVersion: protocol.ProtocolVersion,
		ClientCapabilities: &protocol.ClientCapabilities{
			Features:       map[string]protocol.FeaturePreference{protocol.FeatureSubagents: {Enabled: true}},
			InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion},
		},
	})
	first, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	})
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "sibling restart",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "delegate both siblings"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	bCompleted := make(chan string, 1)
	initialDone := make(chan []protocol.RunEvent, 1)
	go func() {
		var observed []protocol.RunEvent
		for event := range events {
			observed = append(observed, event)
			if event.RunID != started.RunID && event.Event.Type == protocol.StreamSegmentFinished &&
				event.Event.Outcome != nil && event.Event.Outcome.Type == protocol.SegmentCompleted {
				bCompleted <- event.RunID
			}
		}
		initialDone <- observed
	}()
	var bRunID string
	select {
	case bRunID = <-bCompleted:
	case observed := <-initialDone:
		diagnostic, _ := json.Marshal(observed[len(observed)-1].Event.Outcome)
		t.Fatalf("tree stopped before sibling B completed after %d model calls: %s", calls.Load(), diagnostic)
	case <-time.After(5 * time.Second):
		t.Fatal("completed sibling B was held behind running sibling A")
	}
	close(releaseA)
	waitForRunEvents(t, initialDone, "waiting sibling A")
	bBefore, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: bRunID})
	if err != nil {
		t.Fatal(err)
	}
	if bBefore.Status != protocol.RunStatusFinished || bBefore.Outcome == nil || bBefore.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("sibling B before restart = %+v", bBefore)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := restarted.Close(); err != nil {
			t.Error(err)
		}
	})
	pending, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: started.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("restarted tree lost sibling A's question: %+v", pending)
	}
	_, resumedEvents, err := api.ResumeRun(ctx, protocol.ResumeRunRequest{
		RunID: started.RunID,
		Responses: []protocol.InterruptResponse{{
			ItemID:   pending.Data[0].Interrupts[0].ItemID,
			Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseAnswer, Answers: [][]string{{"Yes"}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range waitForRunEvents(t, collectRunEvents(resumedEvents), "resumed sibling tree") {
		if event.RunID == bRunID {
			t.Fatalf("completed sibling B emitted a new event after restart: %+v", event)
		}
	}
	finished, err := api.ListRuns(ctx, protocol.ListRunsRequest{SessionID: session.ID, IncludeDescendants: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(finished.Data) != 3 || calls.Load() != 5 {
		t.Fatalf("restored tree has %d Runs and %d provider calls, want 3 and 5", len(finished.Data), calls.Load())
	}
	for _, value := range finished.Data {
		if value.Status != protocol.RunStatusFinished || value.Outcome == nil || value.Outcome.Type != protocol.OutcomeCompleted {
			t.Fatalf("restored Run did not complete: %+v", value)
		}
	}
	bAfter, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: bRunID})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bBefore, bAfter) {
		t.Fatalf("completed sibling changed across restart: before=%+v after=%+v", bBefore, bAfter)
	}
	pending, err = api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: started.RunID})
	if err != nil || len(pending.Data) != 0 {
		t.Fatalf("completed tree still has pending input: %+v, %v", pending, err)
	}
}

type delegateRestartModel struct{ chat.Model }

func (m delegateRestartModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return testsupport.StreamResponse(m.Call(ctx, request))
}
