package agentexec

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

var errInteractionRunCanceled = errors.New("agentexec: Interaction Run cancellation requested")

type interactionDispatchIdentity struct {
	processID agent.ProcessID
	effectID  agent.EffectID
}

func interactionDispatchKey(request agent.EffectRequest) interactionDispatchIdentity {
	return interactionDispatchIdentity{
		processID: request.ProcessID(),
		effectID:  request.ID(),
	}
}

// beginDispatch binds one Agent-owned Effect attempt to the product Run's
// explicit cancellation plane. Agent Framework deliberately lets an in-flight
// Effect settle before applying a cancellation intent; this adapter-owned
// context gives cooperative model and Tool implementations a chance to produce
// that settlement promptly without changing Framework lifecycle semantics.
func (i *interactionSession) beginDispatch(
	ctx context.Context,
	request agent.EffectRequest,
) (context.Context, func()) {
	bound, cancel := context.WithCancelCause(ctx)
	stopLifetimeBinding := context.AfterFunc(i.lifetime.execution, func() {
		cancel(context.Cause(i.lifetime.execution))
	})
	key := interactionDispatchKey(request)
	i.state.mu.Lock()
	if i.state.rootCancellationRequested || i.inCanceledSubtreeLocked(request.ProcessID()) {
		cancel(errInteractionRunCanceled)
	} else {
		i.state.activeDispatches[key] = activeInteractionDispatch{
			processID: request.ProcessID(),
			cancel:    cancel,
		}
	}
	i.state.mu.Unlock()
	return bound, func() {
		i.state.mu.Lock()
		delete(i.state.activeDispatches, key)
		i.state.mu.Unlock()
		stopLifetimeBinding()
		cancel(nil)
	}
}

func (i *interactionSession) cancelAllDispatches() {
	i.state.mu.Lock()
	i.state.rootCancellationRequested = true
	cancels := make([]context.CancelCauseFunc, 0, len(i.state.activeDispatches))
	for _, dispatch := range i.state.activeDispatches {
		cancels = append(cancels, dispatch.cancel)
	}
	i.state.mu.Unlock()
	for _, cancel := range cancels {
		cancel(errInteractionRunCanceled)
	}
}

func (i *interactionSession) cancelSubtreeDispatches(rootID agent.ProcessID) {
	i.state.mu.Lock()
	i.state.canceledSubtreeRoots[rootID] = struct{}{}
	cancels := make([]context.CancelCauseFunc, 0, len(i.state.activeDispatches))
	for _, dispatch := range i.state.activeDispatches {
		if i.inSubtreeLocked(dispatch.processID, rootID) {
			cancels = append(cancels, dispatch.cancel)
		}
	}
	i.state.mu.Unlock()
	for _, cancel := range cancels {
		cancel(errInteractionRunCanceled)
	}
}

func (i *interactionSession) inCanceledSubtreeLocked(processID agent.ProcessID) bool {
	for rootID := range i.state.canceledSubtreeRoots {
		if i.inSubtreeLocked(processID, rootID) {
			return true
		}
	}
	return false
}

func (i *interactionSession) inSubtreeLocked(
	processID agent.ProcessID,
	rootID agent.ProcessID,
) bool {
	for range len(i.state.delegateChildren) + 1 {
		if processID == rootID {
			return true
		}
		managed := i.state.delegateChildren[processID]
		if managed == nil {
			return false
		}
		processID = managed.identity.parentID
	}
	return false
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
	accepted, deliverErr := process.DeliverSignal(
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

func (i *interactionSession) commitAppliedSteers(
	ctx context.Context,
	member runs.ExecutorMember,
	signalIDs []agent.SignalID,
) error {
	if len(signalIDs) == 0 {
		return nil
	}
	i.state.mu.Lock()
	messages := make([]runs.AppliedSteerMessage, 0, len(signalIDs))
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
			Content: transcript.CloneContent(pending.content), ProjectedItemID: pending.projectedItemID,
		})
	}
	i.state.mu.Unlock()
	if len(messages) > 0 {
		if err := i.commitFact(ctx, member, runs.SteerMessagesApplied{Messages: messages}); err != nil {
			return fmt.Errorf("agentexec: commit applied Interaction steers: %w", err)
		}
	}
	i.state.mu.Lock()
	for _, signalID := range signalIDs {
		delete(i.state.pendingSteers, signalID)
	}
	i.state.mu.Unlock()
	return nil
}
