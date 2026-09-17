package agentexec

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

const modelProcessStopReason = "model call cannot continue"

// awaitDispatchSegment keeps Scope-owned work behind the product transaction
// that opens its next Segment. Scope owns cancellation of the supplied context;
// session release can also unblock the gate without creating another tree owner.
func (i *interactionSession) awaitDispatchSegment(ctx context.Context) error {
	i.state.mu.Lock()
	ready := i.state.dispatchReady
	i.state.mu.Unlock()
	if ready != nil {
		select {
		case <-ready:
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-i.lifetime.releasing.Done():
			return errInteractionReleased
		}
	}
	return context.Cause(ctx)
}

// stopModelProcess ends only the member whose model call cannot continue.
// Scope preserves model errors as unknown external outcomes; Runtime owns the
// decision to stop and retains its provider failure separately.
func (i *interactionSession) stopModelProcess(ctx context.Context, processID agent.ProcessID) error {
	process, found := i.engine.Process(processID)
	if !found {
		return fmt.Errorf("agentexec: model process %s is unavailable", processID)
	}
	controlCtx, cancel := i.lifetime.publicationContext(ctx)
	defer cancel()
	if err := process.RequestCancellation(controlCtx, modelProcessStopReason); err != nil && !errors.Is(err, agent.ErrProcessFinished) {
		return fmt.Errorf("agentexec: stop model process: %w", err)
	}
	return nil
}

func (i *interactionSession) submitSteer(
	ctx context.Context,
	message corechat.Message,
	content []transcript.ContentBlock,
) error {
	if ctx == nil {
		return errors.New("agentexec: steer context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	process := i.state.processHandle()
	if process == nil {
		return runs.ErrExecutorNotLive
	}
	signalID, err := agent.ParseSignalID("steer:" + uuid.NewString())
	if err != nil {
		return fmt.Errorf("agentexec: construct Interaction steer identity: %w", err)
	}
	signal, err := interaction.NewSteerSignal(signalID, message)
	if err != nil {
		return fmt.Errorf("agentexec: construct Interaction steer Signal: %w", err)
	}
	i.state.mu.Lock()
	i.state.pendingSteers[signalID] = pendingInteractionSteer{
		content: transcript.CloneContent(content),
	}
	i.state.mu.Unlock()
	accepted, deliverErr := process.DeliverSignals(
		runExecutionContext(ctx, i.scope, i.start), signal,
	)
	if deliverErr != nil {
		// A context error only reports that the caller stopped waiting. Engine may
		// already have accepted the command, so retain its exact product mapping
		// until ModelInvocation attributes it or the session is released.
		if !errors.Is(deliverErr, context.Canceled) &&
			!errors.Is(deliverErr, context.DeadlineExceeded) {
			i.removePendingSteer(signalID)
		}
		return fmt.Errorf("agentexec: deliver Interaction steer Signal: %w", deliverErr)
	}
	if !accepted {
		i.removePendingSteer(signalID)
		return errors.New("agentexec: Interaction steer Signal was not accepted")
	}
	return nil
}

func (i *interactionSession) removePendingSteer(signalID agent.SignalID) {
	i.state.mu.Lock()
	delete(i.state.pendingSteers, signalID)
	i.state.mu.Unlock()
}

func (i *interactionSession) commitAppliedInputs(
	ctx context.Context,
	member runs.ExecutorMember,
	processID agent.ProcessID,
	signalIDs []agent.SignalID,
) error {
	i.state.mu.Lock()
	messages := make([]runs.AppliedSteerMessage, 0, len(signalIDs)+1)
	seen := make(map[agent.SignalID]struct{}, len(signalIDs))
	for _, signalID := range signalIDs {
		if _, duplicate := seen[signalID]; duplicate {
			i.state.mu.Unlock()
			return fmt.Errorf("agentexec: model attribution repeats steer Signal %s", signalID)
		}
		seen[signalID] = struct{}{}
		pending, found := i.state.pendingSteers[signalID]
		if !found {
			i.state.mu.Unlock()
			return fmt.Errorf("agentexec: model attribution names unknown steer Signal %s", signalID)
		}
		messages = append(messages, runs.AppliedSteerMessage{
			Content: transcript.CloneContent(pending.content),
		})
	}
	var continuation pendingInteractionContinuation
	if pending := i.state.pendingContinuation; pending != nil && pending.processID == processID {
		continuation = pendingInteractionContinuation{
			processID: pending.processID,
			itemID:    pending.itemID,
			content:   transcript.CloneContent(pending.content),
		}
		messages = append(messages, runs.AppliedSteerMessage{
			Content:         transcript.CloneContent(pending.content),
			ProjectedItemID: pending.itemID,
		})
	}
	i.state.mu.Unlock()
	if len(messages) == 0 {
		return nil
	}
	if err := i.commitFact(ctx, member, runs.SteerMessagesApplied{Messages: messages}); err != nil {
		return fmt.Errorf("agentexec: commit applied Interaction inputs: %w", err)
	}
	i.state.mu.Lock()
	for _, signalID := range signalIDs {
		delete(i.state.pendingSteers, signalID)
	}
	if current := i.state.pendingContinuation; current != nil &&
		current.processID == continuation.processID && current.itemID == continuation.itemID {
		i.state.pendingContinuation = nil
	}
	i.state.mu.Unlock()
	return nil
}

func (i *interactionSession) pendingContinuationFor(
	processID agent.ProcessID,
) (pendingInteractionContinuation, bool, error) {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	pending := i.state.pendingContinuation
	if pending == nil || pending.processID != processID {
		return pendingInteractionContinuation{}, false, nil
	}
	if !processID.Valid() || pending.itemID == "" || len(pending.content) == 0 {
		return pendingInteractionContinuation{}, false, errors.New("agentexec: invalid pending continuation input")
	}
	return pendingInteractionContinuation{
		processID: pending.processID,
		itemID:    pending.itemID,
		content:   transcript.CloneContent(pending.content),
	}, true, nil
}
