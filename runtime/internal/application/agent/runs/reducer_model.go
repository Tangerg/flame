package runs

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	corechat "github.com/Tangerg/scope/core/chat"
)

type modelCall struct {
	startedAt time.Time
	purpose   run.ModelCallPurpose
}

func (r *reducer) startModelCall(started ModelCallStarted) (factReduction, error) {
	if err := started.Purpose.Validate(); err != nil {
		return factReduction{}, fmt.Errorf("%w: %w", errExecutorContract, err)
	}
	if err := ValidateInvocation(started.CallID); err != nil {
		return factReduction{}, fmt.Errorf("%w: model call start: %w", errExecutorContract, err)
	}
	if _, duplicate := r.modelCalls[started.CallID]; duplicate {
		return factReduction{}, fmt.Errorf("%w: model call %q started more than once", errExecutorContract, started.CallID)
	}
	startedAt := r.now()
	r.modelCalls[started.CallID] = modelCall{startedAt: startedAt, purpose: started.Purpose}
	if started.Purpose == run.ModelConversation {
		r.modelBoundaryClosed = false
	}
	return factReduction{
		events: []ProjectionEvent{SegmentProgressed{Progress: Progress{Activity: CallingModelActivity{}}}},
		modelInvocations: []ModelInvocationCommit{{
			CallID: started.CallID, SegmentID: r.cfg.Opened.ActiveSegmentID(),
			State: modelref.InvocationStarted, Purpose: started.Purpose, StartedAt: startedAt,
		}},
	}, nil
}

func (r *reducer) completeModelCall(completed ModelCallCompleted) (factReduction, error) {
	if err := ValidateInvocation(completed.CallID); err != nil {
		return factReduction{}, fmt.Errorf("%w: model call completion: %w", errExecutorContract, err)
	}
	call, started := r.modelCalls[completed.CallID]
	startedAt := call.startedAt
	if !started {
		return factReduction{}, fmt.Errorf("%w: model call %q completed without a start", errExecutorContract, completed.CallID)
	}
	finishedAt := r.now()
	if finishedAt.Before(startedAt) {
		return factReduction{}, fmt.Errorf(
			"%w: model call %q completion precedes its start",
			errExecutorContract,
			completed.CallID,
		)
	}
	if call.purpose == run.ModelIndependent && (completed.Message != nil || completed.ContextTokens != 0 || completed.FirstOutputLatencyMillis != nil) {
		return factReduction{}, fmt.Errorf("%w: independent model completion carries conversation state", errExecutorContract)
	}
	delete(r.modelCalls, completed.CallID)
	var events []ProjectionEvent
	var conversationMessages []corechat.Message
	if completed.Message != nil {
		message := *completed.Message
		var err error
		events, err = r.completeModelMessage(message)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: model call completion: %w", errExecutorContract, err)
		}
		conversationMessages = r.rootConversationMessages(message)
	}
	if call.purpose == run.ModelConversation {
		r.modelBoundaryClosed = true
		previews, err := r.retainIncompleteModelOutput(ModelObservation{})
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: retain unsettled model output: %w", errReducerInvariant, err)
		}
		events = append(events, previews...)
	}
	progressEvents, err := r.modelUsageProgress(completed)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: model call usage: %w", errExecutorContract, err)
	}
	progressed, err := r.progressedRun(finishedAt)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: model call progress: %w", errExecutorContract, err)
	}
	return factReduction{
		events:               append(events, progressEvents...),
		conversationMessages: conversationMessages,
		modelInvocations: []ModelInvocationCommit{{
			CallID: completed.CallID, SegmentID: r.cfg.Opened.ActiveSegmentID(),
			State: modelref.InvocationCompleted, Purpose: call.purpose, StartedAt: startedAt, FinishedAt: finishedAt, Usage: completed.ReportedUsage,
			FirstOutputLatencyMillis: completed.FirstOutputLatencyMillis,
		}},
		progress: &progressed,
	}, nil
}

func (r *reducer) failModelCall(failed ModelCallFailed) (factReduction, error) {
	if err := ValidateInvocation(failed.CallID); err != nil {
		return factReduction{}, fmt.Errorf("%w: model call failure: %w", errExecutorContract, err)
	}
	call, started := r.modelCalls[failed.CallID]
	if !started {
		return factReduction{}, fmt.Errorf("%w: model call %q failed without a start", errExecutorContract, failed.CallID)
	}
	finishedAt := r.now()
	if finishedAt.Before(call.startedAt) {
		return factReduction{}, fmt.Errorf("%w: model call %q failure precedes its start", errExecutorContract, failed.CallID)
	}
	if err := failed.validateFor(call.purpose); err != nil {
		return factReduction{}, err
	}
	var events []ProjectionEvent
	if call.purpose == run.ModelConversation {
		retained, err := r.retainIncompleteModelOutput(failed.Observation)
		if err != nil {
			return factReduction{}, err
		}
		events = retained
	}
	var usage *corechat.Usage
	var progressed *run.Run
	if failed.Accounting != nil {
		progressEvents, err := r.modelUsageProgress(*failed.Accounting)
		if err != nil {
			return factReduction{}, err
		}
		events = append(events, progressEvents...)
		value, err := r.progressedRun(finishedAt)
		if err != nil {
			return factReduction{}, err
		}
		progressed = &value
		usage = failed.Accounting.ReportedUsage
	}

	delete(r.modelCalls, failed.CallID)
	if call.purpose == run.ModelConversation {
		r.modelBoundaryClosed = true
	}
	return factReduction{
		progress: progressed,
		events:   append(events, SegmentProgressed{Progress: Progress{Activity: FailedModelCallActivity{}}}),
		modelInvocations: []ModelInvocationCommit{{
			CallID: failed.CallID, SegmentID: r.cfg.Opened.ActiveSegmentID(),
			State: modelref.InvocationFailed, Purpose: call.purpose, StartedAt: call.startedAt, FinishedAt: finishedAt,
			Usage: usage, KnownResponse: failed.KnownResponse.Clone(),
			FirstOutputLatencyMillis: failed.FirstOutputLatencyMillis,
		}},
	}, nil
}

func (r *reducer) closeLostModelCalls(ending run.Ending) ([]ModelInvocationCommit, error) {
	if _, lost := ending.(run.Lost); !lost {
		return nil, nil
	}
	finishedAt := r.now()
	callIDs := slices.Sorted(maps.Keys(r.modelCalls))
	invocations := make([]ModelInvocationCommit, 0, len(callIDs))
	for _, callID := range callIDs {
		call := r.modelCalls[callID]
		startedAt := call.startedAt
		if finishedAt.Before(startedAt) {
			return nil, fmt.Errorf("model call %q loss precedes its start", callID)
		}
		invocations = append(invocations, ModelInvocationCommit{
			CallID: callID, SegmentID: r.cfg.Opened.ActiveSegmentID(),
			State: modelref.InvocationUnknown, Purpose: call.purpose, StartedAt: startedAt, FinishedAt: finishedAt,
		})
	}
	clear(r.modelCalls)
	return invocations, nil
}
