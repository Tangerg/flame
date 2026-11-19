package bootstrap

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/scope/core/chat"
)

func TestCompactedRunKeepsItsQuestionAndUsageAcrossRestart(t *testing.T) {
	const questionCallID = "call_question_after_compaction"
	const answer = "Continue using the compacted context."
	const waitingContext = 200
	const resumedContext = 220
	const resumedTokens = 5
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	longRun := &longContextModel{}
	opening := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		_, summaries, _, _ := longRun.Snapshot()
		if summaries == 0 || isCompactionRequest(request) {
			return longRun.Call(ctx, request)
		}
		if !requestContainsText(request, longContextCompactionSummary) {
			return nil, errors.New("provider did not receive the committed summary")
		}
		response := toolCallResponse(chat.ToolCall{
			ID: questionCallID, Name: string(builtin.AskUser),
			Arguments: `{"questions":[{"question":"Continue after compaction?"}]}`,
		})
		return chat.NewResponse(response.Output, &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: waitingContext, OutputTokens: 10}})
	})}
	first, api := openProtocolRuntime(t, opening)
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "waiting after compaction",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Compact one long turn, then wait for my answer."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	opened := waitForRunEvents(t, collectRunEvents(events), "compaction and question")
	if summary, found := compactionSummaryFromEvents(opened); !found || summary != longContextCompactionSummary {
		t.Fatalf("compaction summary = %q, found=%t", summary, found)
	}
	readRun := protocol.GetRunRequest{RunID: started.RunID}
	readCalls := protocol.ListModelInvocationsRequest{RunID: started.RunID}
	readInterrupts := protocol.ListInterruptsRequest{RootRunID: started.RunID}
	waiting, err := api.GetRun(ctx, readRun)
	if err != nil || waiting.Status != protocol.RunStatusWaiting || waiting.Outcome != nil {
		t.Fatalf("before restart: Run = %+v, error = %v", waiting, err)
	}
	before, err := api.ListModelInvocations(ctx, readCalls)
	mainCalls, summaries, _, _ := longRun.Snapshot()
	wantCalls := mainCalls + 2
	if err != nil || summaries != 1 || len(before.Data) != wantCalls || waiting.Metrics.Steps != wantCalls || waiting.ContextTokens != waitingContext || waiting.Metrics.Usage == nil {
		t.Fatalf("before restart: main calls before summary=%d, summaries=%d, model records=%v, metrics=%+v, context=%d, error=%v",
			mainCalls, summaries, before, waiting.Metrics, waiting.ContextTokens, err)
	}
	pending, err := api.ListInterrupts(ctx, readInterrupts)
	if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("before restart: pending = %+v, error = %v", pending, err)
	}
	question := pending.Data[0].Interrupts[0]
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	var callsAfterRestart atomic.Int64
	resuming := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		callsAfterRestart.Add(1)
		if !requestContainsText(request, longContextCompactionSummary) {
			return nil, errors.New("resumed request lost the committed summary")
		}
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.Kind != chat.PartToolResult || part.ToolResult.ID != questionCallID {
					continue
				}
				output, text := part.ToolResult.Output.Text()
				if part.ToolResult.IsError || !text || !strings.Contains(output, answer) {
					return nil, errors.New("resumed request lost the accepted answer")
				}
				response := completedTextResponse("continued once after the question")
				return chat.NewResponse(response.Output, &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: resumedContext, OutputTokens: resumedTokens}})
			}
		}
		return nil, errors.New("resumed request lost the original question result")
	})}
	second, restored := openProtocolRuntime(t, resuming)
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Error(err)
		}
	})
	parked, err := restored.GetRun(ctx, readRun)
	if err != nil || !reflect.DeepEqual(parked, waiting) {
		t.Fatalf("after restart: Run = %+v, want original waiting Run %+v, error = %v", parked, waiting, err)
	}
	remaining, err := restored.ListInterrupts(ctx, readInterrupts)
	if err != nil || !reflect.DeepEqual(remaining, pending) {
		t.Fatalf("after restart: pending = %+v, want original question %+v, error = %v", remaining, pending, err)
	}
	after, err := restored.ListModelInvocations(ctx, readCalls)
	if err != nil || !reflect.DeepEqual(after, before) || callsAfterRestart.Load() != 0 {
		t.Fatalf("after restart: original model records changed or requests were replayed, calls = %d, error = %v", callsAfterRestart.Load(), err)
	}
	resumed, events, err := restored.ResumeRun(ctx, protocol.ResumeRunRequest{
		RunID: started.RunID,
		Responses: []protocol.InterruptResponse{{
			ItemID:   question.ItemID,
			Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseAnswer, Answers: [][]string{{answer}}},
		}},
	})
	if err != nil || resumed.RunID != started.RunID || resumed.SegmentID == started.SegmentID {
		t.Fatalf("resumed = %+v, error = %v", resumed, err)
	}
	waitForRunEvents(t, collectRunEvents(events), "original Run after the answer")
	finished, err := restored.GetRun(ctx, readRun)
	if err != nil || finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted ||
		finished.Metrics.Steps != wantCalls+1 || callsAfterRestart.Load() != 1 {
		t.Fatalf("after answer: Run = %+v, new model requests = %d, error = %v", finished, callsAfterRestart.Load(), err)
	}
	usage := finished.Metrics.Usage
	if usage == nil || usage.InputTokens != waiting.Metrics.Usage.InputTokens+resumedContext ||
		usage.OutputTokens != waiting.Metrics.Usage.OutputTokens+resumedTokens || finished.ContextTokens != resumedContext {
		t.Fatalf("resumed call usage = %+v, context = %d, waiting usage = %+v", usage, finished.ContextTokens, waiting.Metrics.Usage)
	}
}
