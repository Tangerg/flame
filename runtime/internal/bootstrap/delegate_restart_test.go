package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

func TestProtocolContinuesWaitingTreeBesideCompletedSiblingAfterRestart(t *testing.T) {
	waiting := chat.ToolCall{ID: "delegate_a", Name: "delegate_task", Arguments: `{"summary":"A","instructions":"waiting sibling A"}`}
	completed := chat.ToolCall{ID: "delegate_b", Name: "delegate_task", Arguments: `{"summary":"B","instructions":"completed sibling B"}`}
	for _, test := range []struct {
		name          string
		calls         []chat.ToolCall
		cancelWaiting bool
	}{
		{name: "answer with completed successor", calls: []chat.ToolCall{waiting, completed}},
		{name: "answer with completed predecessor", calls: []chat.ToolCall{completed, waiting}},
		{name: "cancel with completed successor", calls: []chat.ToolCall{waiting, completed}, cancelWaiting: true},
		{name: "cancel with completed predecessor", calls: []chat.ToolCall{completed, waiting}, cancelWaiting: true},
	} {
		t.Run(test.name, func(t *testing.T) { testProtocolSiblingRestart(t, test.calls, test.cancelWaiting) })
	}
}

func testProtocolSiblingRestart(t *testing.T, delegateCalls []chat.ToolCall, cancelWaiting bool) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	releaseA := make(chan struct{})
	var calls atomic.Int32
	model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		calls.Add(1)
		var hasResult, waitingChild, completedChild bool
		var resultIDs []string
		for _, message := range request.Messages {
			hasResult = hasResult || message.Role == chat.RoleTool
			for _, part := range message.Parts {
				if part.ToolResult != nil {
					resultIDs = append(resultIDs, part.ToolResult.ID)
				}
			}
			if message.Role == chat.RoleUser {
				waitingChild = waitingChild || strings.Contains(message.Text(), "waiting sibling A")
				completedChild = completedChild || strings.Contains(message.Text(), "completed sibling B")
			}
		}
		message := chat.NewAssistantMessage(chat.NewTextPart("continued"))
		finish := chat.FinishReasonStop
		switch {
		case hasResult:
			if !waitingChild && !completedChild &&
				!slices.Equal(resultIDs, []string{delegateCalls[0].ID, delegateCalls[1].ID}) {
				return nil, fmt.Errorf("continued parent received tool results %v in place of both ordered siblings", resultIDs)
			}
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
	wantCalls := int32(5)
	canceledRunID := ""
	if cancelWaiting {
		canceledRunID = pending.Data[0].Interrupts[0].RunID
		if _, err := api.CancelRun(ctx, protocol.CancelRunRequest{RunID: canceledRunID, Reason: "skip sibling A"}); err != nil {
			t.Fatal(err)
		}
		waitForProtocolRunTerminal(t, ctx, api, started.RunID)
		wantCalls = 4
	} else {
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
	}
	finished, err := api.ListRuns(ctx, protocol.ListRunsRequest{SessionID: session.ID, IncludeDescendants: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(finished.Data) != 3 {
		t.Fatalf("restored tree has %d Runs, want 3", len(finished.Data))
	}
	for _, value := range finished.Data {
		wantOutcome := protocol.OutcomeCompleted
		if value.ID == canceledRunID {
			wantOutcome = protocol.OutcomeCanceled
		}
		if value.Status != protocol.RunStatusFinished || value.Outcome == nil || value.Outcome.Type != wantOutcome {
			diagnostic, _ := json.Marshal(value)
			t.Fatalf("restored Run did not finish as %s: %s", wantOutcome, diagnostic)
		}
	}
	if calls.Load() != wantCalls {
		t.Fatalf("restored tree made %d provider calls, want %d", calls.Load(), wantCalls)
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

func waitForProtocolRunTerminal(t *testing.T, ctx context.Context, api *delivery.Handler, runID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		value, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: runID})
		if err != nil {
			t.Fatal(err)
		}
		if value.Status == protocol.RunStatusFinished {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("continued Run did not reach its terminal state")
		}
	}
}

type delegateRestartModel struct{ chat.Model }

func (m delegateRestartModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return testsupport.StreamResponse(m.Call(ctx, request))
}
