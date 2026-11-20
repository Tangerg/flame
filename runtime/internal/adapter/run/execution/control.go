package execution

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
)

const modelProcessStopReason = "model call cannot continue"
const cancellationControlPrefix = "run cancel command "

func steerSignalID(itemID identity.ItemID) (scoperun.SignalID, error) {
	if err := itemID.Validate(); err != nil {
		return scoperun.SignalID{}, err
	}
	return scoperun.ParseSignalID("steer:" + itemID.String())
}

// Application retains the exact user note. Scope receives a bounded diagnostic
// in a separate namespace so that user text cannot impersonate an executor stop.
func scopeCancellationReason(reason string) string {
	return run.Diagnostic(errors.New("run cancellation: " + reason))
}

func cancellationFromTermination(termination scoperun.Termination) (runs.CancellationID, bool) {
	switch termination.Cause() {
	case scoperun.TerminationCauseHostCancellation, scoperun.TerminationCauseParentCancellation:
	default:
		return runs.CancellationID{}, false
	}
	token, ok := strings.CutPrefix(termination.Reason(), cancellationControlPrefix)
	if !ok {
		return runs.CancellationID{}, false
	}
	return runs.ParseCancellationID(token)
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
func (i *interactionSession) stopModelProcess(ctx context.Context, processID scoperun.ProcessID) error {
	process, found := i.engine.Process(processID)
	if !found {
		return fmt.Errorf("execution: model process %s is unavailable", processID)
	}
	controlCtx, cancel := i.lifetime.publicationContext(ctx)
	defer cancel()
	if err := process.Cancel(controlCtx, modelProcessStopReason); err != nil && !errors.Is(err, scoperun.ErrProcessFinished) {
		return fmt.Errorf("execution: stop model process: %w", err)
	}
	return nil
}

func (i *interactionSession) submitSteer(
	ctx context.Context,
	prepared runs.PreparedSteer,
) (identity.ItemID, error) {
	if err := ctx.Err(); err != nil {
		return identity.ItemID{}, err
	}
	process := i.state.processHandle()
	if process == nil {
		return identity.ItemID{}, runs.ErrExecutorNotLive
	}
	if command, bound := prepared.Command(); bound && (command.SessionID != i.start.SessionID || command.RunID != i.start.RunID) {
		return identity.ItemID{}, errors.New("execution: steer command addresses another Run")
	}
	signalID, signal, err := steerSignal(prepared)
	if err != nil {
		return identity.ItemID{}, err
	}
	// Rejected duplicate dispatch must never erase or replace the original
	// mapping while Scope is committing or applying that same Signal.
	release, err := i.steerAdmissions.Acquire(ctx, signalID.String())
	if err != nil {
		return identity.ItemID{}, err
	}
	defer release()
	if err := i.deliverSteer(ctx, process, signalID, signal, prepared); err != nil {
		return identity.ItemID{}, err
	}
	return prepared.ItemID(), nil
}

func steerSignal(prepared runs.PreparedSteer) (scoperun.SignalID, scoperun.Signal, error) {
	signalID, err := steerSignalID(prepared.ItemID())
	if err != nil {
		return scoperun.SignalID{}, scoperun.Signal{}, fmt.Errorf("execution: construct Interaction steer identity: %w", err)
	}
	message, err := runs.MaterializeUserMessage(prepared.Input())
	if err != nil {
		return scoperun.SignalID{}, scoperun.Signal{}, err
	}
	signal, err := materializeSteerSignal(signalID, message)
	if err != nil {
		return scoperun.SignalID{}, scoperun.Signal{}, fmt.Errorf("execution: construct Interaction steer Signal: %w", err)
	}
	return signalID, signal, nil
}

// deliverSteer delivers a steer Signal exactly once. A Signal Scope already
// admitted is a duplicate whose receipt is that admission; a pending one must
// be the same steer, or the command key was reused.
func (i *interactionSession) deliverSteer(ctx context.Context, process *scoperun.Handle[scoperun.Payload], signalID scoperun.SignalID, signal scoperun.Signal, prepared runs.PreparedSteer) error {
	i.state.mu.Lock()
	prior, present := i.state.pendingSteers[signalID]
	i.state.mu.Unlock()
	if present && !prior.Equal(prepared) {
		return runs.ErrCommandKeyConflict
	}
	committed, err := i.committedTree(ctx, process.ID().Root())
	if err != nil {
		return err
	}
	matched, err := exactSignalAdmission(committed, process.ID(), signal)
	if err != nil {
		return errors.Join(runs.ErrCommandKeyConflict, err)
	}
	if matched {
		return nil
	}
	if !present {
		i.state.mu.Lock()
		i.state.pendingSteers[signalID] = prepared
		i.state.mu.Unlock()
	}
	accepted, deliverErr := process.DeliverSignals(
		runExecutionContext(ctx, i.scope, i.start), signal,
	)
	if deliverErr != nil {
		// A context error only reports that the caller stopped waiting. Engine may
		// already have accepted the command, so retain its exact product mapping
		// until a committed model request attributes it or the session is released.
		if !present && !errors.Is(deliverErr, context.Canceled) &&
			!errors.Is(deliverErr, context.DeadlineExceeded) {
			i.removePendingSteer(signalID)
		}
		return fmt.Errorf("execution: deliver Interaction steer Signal: %w", deliverErr)
	}
	if !accepted {
		tree, err := i.committedTree(ctx, process.ID().Root())
		if err != nil {
			return err
		}
		matched, err := exactSignalAdmission(tree, process.ID(), signal)
		if err != nil {
			return err
		}
		if !matched {
			return errors.New("execution: steer duplicate has no exact admission receipt")
		}
	}
	return nil
}

func (i *interactionSession) removePendingSteer(signalID scoperun.SignalID) {
	i.state.mu.Lock()
	delete(i.state.pendingSteers, signalID)
	i.state.mu.Unlock()
}

func (i *interactionSession) commitAppliedInputs(
	ctx context.Context,
	member runs.ExecutorMember,
	processID scoperun.ProcessID,
	signalIDs []scoperun.SignalID,
) error {
	i.state.mu.Lock()
	var resumeInputs []runs.CommittedUserInput
	if processID == i.rootID {
		resumeInputs = i.state.pendingResumeInputs
	}
	messages := make([]runs.AppliedSteerMessage, 0, len(signalIDs)+len(resumeInputs))
	seen := make(map[scoperun.SignalID]struct{}, len(signalIDs))
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
			Content: pending.Input(),
			ItemID:  pending.ItemID(),
		})
	}
	for _, input := range resumeInputs {
		messages = append(messages, runs.AppliedSteerMessage{
			Content:          transcript.CloneContent(input.Content),
			ItemID:           input.ItemID,
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
	if len(resumeInputs) != 0 {
		i.state.pendingResumeInputs = slices.Delete(i.state.pendingResumeInputs, 0, len(resumeInputs))
	}
	i.state.mu.Unlock()
	return nil
}

func (i *interactionSession) pendingResumeMessages(processID scoperun.ProcessID) ([]corechat.Message, error) {
	if processID != i.rootID {
		return nil, nil
	}
	i.state.mu.Lock()
	inputs := slices.Clone(i.state.pendingResumeInputs)
	i.state.mu.Unlock()
	messages := make([]corechat.Message, 0, len(inputs))
	for _, input := range inputs {
		message, err := runs.MaterializeUserMessage(input.Content)
		if err != nil {
			return nil, fmt.Errorf("execution: materialize accepted Resume input %q: %w", input.ItemID, err)
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func materializeSteerSignal(id scoperun.SignalID, message corechat.Message) (scoperun.Signal, error) {
	message = message.Clone()
	if err := message.Metadata.Set(steerMetadataKey, id); err != nil {
		return scoperun.Signal{}, err
	}
	return interaction.NewSteerSignal(id, message)
}
