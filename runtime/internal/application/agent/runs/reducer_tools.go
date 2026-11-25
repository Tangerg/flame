package runs

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
)

func newToolInvocation(source tool.Ref, name string, arguments tool.Arguments, result *tool.Result) *transcript.ToolInvocation {
	return &transcript.ToolInvocation{
		Name:   name,
		Source: source,
		Input:  transcript.AdmittedInput(arguments),
		Result: result,
	}
}

type openTool struct {
	callID            string
	sourceCallID      string
	modelCallSequence uint64
	toolCallIndex     uint32
	item              transcript.Item
	attemptInvocation transcript.ToolInvocation
	attemptStartedAt  time.Time
	finishedAt        time.Time
}

func (t *openTool) startedAs(start ToolCallStarted) bool {
	call, _ := t.item.ToolCall()
	return t.modelCallSequence == start.ModelCallSequence && t.toolCallIndex == start.ToolCallIndex &&
		t.sourceCallID == start.SourceCallID && call.Invocation.MatchesSource(start.ToolName, start.Source) &&
		t.attemptInvocation.Input.Equal(start.Input)
}

type toolPosition struct {
	modelCallSequence uint64
	toolCallIndex     uint32
}

func (r *reducer) prepareToolCall(prepared ToolCallPrepared) (factReduction, error) {
	if err := prepared.Caller.Validate(); err != nil {
		return factReduction{}, err
	}
	caller := prepared.Caller.Snapshot()
	if caller.Invocation.ID != prepared.Start.CallID || !prepared.Start.MatchesCapturedRef(caller.Tool) {
		return factReduction{}, fmt.Errorf("%w: prepared Tool caller differs from execution", errExecutorContract)
	}
	prepared.Start.Source = caller.Tool
	reduced, err := r.startToolCall(prepared.Start)
	if err != nil {
		return reduced, err
	}
	if len(reduced.toolInvocations) != 1 {
		return factReduction{}, fmt.Errorf("%w: prepared Tool has no invocation", errExecutorContract)
	}
	reduced.toolInvocations[0].Caller = new(prepared.Caller)
	reduced.executionContext = new(prepared.Context.Clone())
	return reduced, nil
}

func (r *reducer) adoptToolAttempt(started ToolCallStarted, attempt OpenToolInvocation) error {
	if r.resume == nil || attempt.CallID != started.CallID || r.resume.callItems[started.CallID].ID() != attempt.ItemID ||
		attempt.SessionID != r.cfg.Opened.SessionID() || started.ModelCallSequence == 0 {
		return fmt.Errorf("%w: restored Tool attempt %q differs from its durable Item", errExecutorContract, started.CallID)
	}
	if _, err := r.toolStart(started); err != nil {
		return err
	}
	ref, _ := r.tools.get(started.CallID)
	ref.attemptStartedAt = attempt.StartedAt
	delete(r.restoredToolAttempts, started.CallID)
	return nil
}

func (r *reducer) startToolCall(started ToolCallStarted) (factReduction, error) {
	events, err := r.toolStart(started)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: tool call start: %w", errExecutorContract, err)
	}
	ref, _ := r.tools.get(started.CallID)
	if ref == nil {
		return factReduction{}, fmt.Errorf("%w: started Tool %q has no open projection", errReducerInvariant, started.CallID)
	}
	reduced := factReduction{events: events, items: []transcript.Item{ref.item}}
	if ref.modelCallSequence > 0 {
		reduced.toolInvocations = []ToolInvocationCommit{{
			CallID: ref.callID, ItemID: ref.item.ID(), SegmentID: r.cfg.Opened.ActiveSegmentID(),
			State: ToolInvocationStarted, StartedAt: ref.attemptStartedAt,
		}}
	}
	return reduced, nil
}

func (r *reducer) finishToolCall(finished ToolCallFinished) (factReduction, error) {
	events, invocations, err := r.toolEnd(finished)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: tool call end: %w", errExecutorContract, err)
	}
	return factReduction{
		events:          events,
		toolInvocations: invocations,
	}, nil
}

func closedToolInvocationCommits(segmentID identity.SegmentID, tools []*openTool) []ToolInvocationCommit {
	commits := make([]ToolInvocationCommit, 0, len(tools))
	for _, ref := range tools {
		if ref == nil || ref.modelCallSequence == 0 || ref.finishedAt.IsZero() {
			continue
		}
		state := ToolInvocationIncomplete
		commits = append(commits, ToolInvocationCommit{
			CallID: ref.callID, ItemID: ref.item.ID(), SegmentID: segmentID,
			State: state, StartedAt: ref.attemptStartedAt, FinishedAt: ref.finishedAt,
		})
	}
	return commits
}

func (r *reducer) abandonUnconsumedResumeTools() ([]ProjectionEvent, error) {
	if r.resume == nil {
		return nil, nil
	}
	remaining := r.resume.remainingDrainedTools()
	events := make([]ProjectionEvent, 0, len(remaining))
	for _, drained := range remaining {
		resumed := r.resume.callItems[drained.CallID]
		ref := &openTool{
			callID:       drained.CallID,
			sourceCallID: drained.SourceCallID,
			item:         resumed,
		}
		completed, err := r.abandonUnstartedToolItem(ref)
		if err != nil {
			return nil, err
		}
		events = append(events, completed)
		r.resume.consumeToolCall(drained.CallID)
	}
	return events, nil
}

func (r *reducer) finishToolResults(batch ToolResultsCommitted) (factReduction, error) {
	if err := batch.validate(); err != nil {
		return factReduction{}, err
	}
	var reduced factReduction
	for _, start := range batch.Starts {
		if ref, open := r.tools.get(start.CallID); open {
			if !ref.startedAs(start) {
				return factReduction{}, errors.New("runs: result publication differs from its open call")
			}
			continue
		}
		started, err := r.startToolCall(start)
		if err != nil {
			return factReduction{}, err
		}
		reduced.events = append(reduced.events, started.events...)
	}
	for _, result := range batch.Results {
		finished, err := r.finishToolCall(result)
		if err != nil {
			return factReduction{}, err
		}
		reduced.toolResults = append(reduced.toolResults, result.ModelResult.Clone())
		reduced.events = append(reduced.events, finished.events...)
		reduced.toolInvocations = append(reduced.toolInvocations, finished.toolInvocations...)
	}
	if len(reduced.toolInvocations) != len(batch.Results) {
		return factReduction{}, errors.New("runs: result publication did not settle its entire call set")
	}
	if len(batch.ModelResults) > 0 {
		reduced.conversationMessages = r.rootConversationMessages(corechat.NewToolMessage(batch.ModelResults...))
	}
	reduced.resultPublication = &batch.Publication
	return reduced, nil
}
