package execution

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

const modelProcessStopReason = "model call cannot continue"

func steerSignalID(itemID string) (agent.SignalID, error) {
	if err := identity.ValidateItem(itemID); err != nil {
		return agent.SignalID{}, err
	}
	return agent.ParseSignalID("steer:" + itemID)
}

func (p pendingInteractionSteer) matches(prepared runs.PreparedSteer) bool {
	if p.itemID != prepared.ItemID || !slices.EqualFunc(p.content, prepared.Input, transcript.ContentBlock.Equal) {
		return false
	}
	if (p.acceptance == nil) != (prepared.Acceptance == nil) {
		return false
	}
	return p.acceptance == nil || p.acceptance.Equal(*prepared.Acceptance)
}

// Application retains the exact user note. Scope receives a bounded diagnostic
// in a separate namespace so that user text cannot impersonate an executor stop.
func scopeCancellationReason(reason string) string {
	return boundDiagnostic("run cancellation: "+reason, maximumExecutorDiagnosticBytes)
}

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
// Scope preserves unmarked model errors as unknown external outcomes; Runtime
// owns the decision to stop and retains its provider failure separately.
func (i *interactionSession) stopModelProcess(ctx context.Context, processID agent.ProcessID) error {
	process, found := i.engine.Process(processID)
	if !found {
		return fmt.Errorf("execution: model process %s is unavailable", processID)
	}
	controlCtx, cancel := i.lifetime.publicationContext(ctx)
	defer cancel()
	if err := process.RequestCancellation(controlCtx, modelProcessStopReason); err != nil && !errors.Is(err, agent.ErrProcessFinished) {
		return fmt.Errorf("execution: stop model process: %w", err)
	}
	return nil
}

func (i *interactionSession) submitSteer(
	ctx context.Context,
	message corechat.Message,
	prepared runs.PreparedSteer,
) (string, error) {
	if ctx == nil {
		return "", errors.New("execution: steer context is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	process := i.state.processHandle()
	if process == nil {
		return "", runs.ErrExecutorNotLive
	}
	itemID := prepared.ItemID
	if err := identity.ValidateItem(itemID); err != nil {
		return "", err
	}
	if acceptance := prepared.Acceptance; acceptance != nil {
		if err := acceptance.Validate(); err != nil {
			return "", err
		}
		expected := acceptance.Clone()
		expected.Operation, expected.SessionID, expected.UserItemID, expected.SteerInput = runs.CommandSteerInput, i.start.SessionID, itemID, prepared.Input
		if !acceptance.Equal(expected) {
			return "", errors.New("execution: steer acceptance does not name its input owner")
		}
	}
	signalID, err := steerSignalID(itemID)
	if err != nil {
		return "", fmt.Errorf("execution: construct Interaction steer identity: %w", err)
	}
	signal, err := interaction.NewSteerSignal(signalID, message)
	if err != nil {
		return "", fmt.Errorf("execution: construct Interaction steer Signal: %w", err)
	}
	// Rejected duplicate dispatch must never erase or replace the original
	// mapping while Scope is committing or applying that same Signal.
	release, err := i.steerAdmissions.Acquire(ctx, signalID.String())
	if err != nil {
		return "", err
	}
	defer release()
	i.state.mu.Lock()
	prior, present := i.state.pendingSteers[signalID]
	i.state.mu.Unlock()
	if present && !prior.matches(prepared) {
		return "", runs.ErrCommandKeyConflict
	}
	committed, err := i.committedTree(ctx, process.Relation().RootID())
	if err != nil {
		return "", err
	}
	for _, snapshot := range committed.ProcessSnapshots() {
		if snapshot.Relation().ProcessID() != process.Relation().ProcessID() {
			continue
		}
		for _, receipt := range snapshot.SignalReceipts() {
			if receipt.ID() != signalID {
				continue
			}
			if !receipt.Matches(signal) {
				return "", runs.ErrCommandKeyConflict
			}
			return itemID, nil
		}
	}
	if !present {
		i.state.mu.Lock()
		i.state.pendingSteers[signalID] = pendingInteractionSteer{
			itemID: itemID, content: transcript.CloneContent(prepared.Input), acceptance: prepared.Acceptance,
		}
		i.state.mu.Unlock()
	}
	accepted, deliverErr := process.DeliverSignals(
		runExecutionContext(ctx, i.scope, i.start), signal,
	)
	if deliverErr != nil {
		// A context error only reports that the caller stopped waiting. Engine may
		// already have accepted the command, so retain its exact product mapping
		// until ModelInvocation attributes it or the session is released.
		if !present && !errors.Is(deliverErr, context.Canceled) &&
			!errors.Is(deliverErr, context.DeadlineExceeded) {
			i.removePendingSteer(signalID)
		}
		return "", fmt.Errorf("execution: deliver Interaction steer Signal: %w", deliverErr)
	}
	if !accepted {
		tree, err := i.committedTree(ctx, process.Relation().RootID())
		if err != nil {
			return "", err
		}
		matched := false
		for _, snapshot := range tree.ProcessSnapshots() {
			if snapshot.Relation().ProcessID() != process.Relation().ProcessID() {
				continue
			}
			for _, receipt := range snapshot.SignalReceipts() {
				if receipt.ID() == signalID && receipt.Matches(signal) {
					matched = true
				}
			}
		}
		if !matched {
			return "", errors.New("execution: steer duplicate has no exact admission receipt")
		}
	}
	return itemID, nil
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
			return fmt.Errorf("execution: model attribution repeats steer Signal %s", signalID)
		}
		seen[signalID] = struct{}{}
		pending, found := i.state.pendingSteers[signalID]
		if !found {
			i.state.mu.Unlock()
			return fmt.Errorf("execution: model attribution names unknown steer Signal %s", signalID)
		}
		messages = append(messages, runs.AppliedSteerMessage{
			Content: transcript.CloneContent(pending.content),
			ItemID:  pending.itemID,
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
			Content:          transcript.CloneContent(pending.content),
			ItemID:           pending.itemID,
			AlreadyProjected: true,
		})
	}
	i.state.mu.Unlock()
	if len(messages) == 0 {
		return nil
	}
	if err := i.commitFact(ctx, member, runs.SteerMessagesApplied{Messages: messages}); err != nil {
		return fmt.Errorf("execution: commit applied Interaction inputs: %w", err)
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
		return pendingInteractionContinuation{}, false, errors.New("execution: invalid pending continuation input")
	}
	return pendingInteractionContinuation{
		processID: pending.processID,
		itemID:    pending.itemID,
		content:   transcript.CloneContent(pending.content),
	}, true, nil
}
