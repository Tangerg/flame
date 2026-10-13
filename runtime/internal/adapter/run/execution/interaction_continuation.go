package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"slices"
	"strings"

	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
)

// BeginContinuation converts every validated application answer into an
// Interaction response Signal. It is called only after the fresh Segment
// opening commits, so accepting the Signal cannot start model/tool work ahead
// of the product's durable lifecycle boundary.
func (i *InteractionExecutor) BeginContinuation(
	ctx context.Context,
	ref runs.ExecutorRef,
	answers []runs.InterruptAnswer,
	input *runs.CommittedUserInput,
) error {
	session, err := i.session(ref)
	if err != nil {
		return err
	}
	if beginContinuationErr := session.beginContinuation(); beginContinuationErr != nil {
		return beginContinuationErr
	}
	paused, err := session.pausedProcessIDs()
	if err != nil {
		return fmt.Errorf("execution: inspect paused Interaction members: %w", err)
	}
	prepared, err := session.prepareContinuationAnswers(ctx, answers)
	if err != nil {
		return err
	}
	committedInput, err := session.prepareCommittedContinuationInput(input)
	if err != nil {
		return err
	}
	// The previous Agent Process lifetime includes the human wait. Reset the
	// Segment clock before any answer can make that Process runnable.
	session.segmentClock.start()
	// Deliver every answer while all recipients stay paused, then resume only the
	// surviving members, and release dispatch last. No recipient can execute an
	// answered Tool before the whole set is admitted, so a partial delivery never
	// runs work ahead of the remaining answers.
	if err := session.deliverContinuationAnswers(ctx, prepared, committedInput); err != nil {
		return err
	}
	if err := session.proveContinuationDelivery(ctx, prepared); err != nil {
		return err
	}
	if err := session.resumePausedProcesses(ctx, paused); err != nil {
		return err
	}
	session.continuationAccepted()
	return nil
}

type preparedInteractionAnswer struct {
	process  *agent.Process
	signal   agent.SignalRequest
	admitted bool
}

// prepareContinuationAnswers reconciles the immutable accepted targets against
// the latest acknowledged waits and receipts without advancing execution.
func (i *interactionSession) prepareContinuationAnswers(
	ctx context.Context,
	answers []runs.InterruptAnswer,
) ([]preparedInteractionAnswer, error) {
	i.state.mu.Lock()
	checkpoint := i.state.waitingCheckpoint.Clone()
	i.state.mu.Unlock()
	checkpointState, err := decodeExecutorCheckpoint(checkpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: decode staged Interaction checkpoint: %w", runs.ErrExecutorStateLost, err)
	}
	head, err := i.committedTree(ctx, checkpointState.handoff.rootID)
	if err != nil {
		return nil, err
	}
	retired := i.acceptedRetiredProcesses(checkpointState.handoff, head)
	if err := validateAcceptedPausedTree(checkpointState.handoff, head, retired); err != nil {
		return nil, err
	}
	targets := make(map[agent.WaitID]interactionInputTarget)
	for wait, target := range checkpointState.handoff.inputs {
		if !retired[target.processID] {
			targets[wait] = target
		}
	}
	orderedAnswers := slices.Clone(answers)
	slices.SortFunc(orderedAnswers, func(left, right runs.InterruptAnswer) int {
		if order := strings.Compare(left.MemberID, right.MemberID); order != 0 {
			return order
		}
		return strings.Compare(left.RequestID, right.RequestID)
	})
	if len(orderedAnswers) != len(targets) {
		return nil, fmt.Errorf("%w: %d Interaction answers do not match %d accepted targets", runs.ErrExecutorStateLost, len(orderedAnswers), len(targets))
	}
	currentInputs, err := interaction.PendingToolInputs(head)
	if err != nil {
		return nil, err
	}
	currentByWait := make(map[agent.WaitID]interaction.PendingToolInput)
	for _, pending := range currentInputs {
		if retired[pending.ProcessID()] {
			continue
		}
		target, found := targets[pending.WaitID()]
		if !found || target.processID != pending.ProcessID() {
			return nil, fmt.Errorf("%w: current input differs from its accepted target", runs.ErrExecutorStateLost)
		}
		currentByWait[pending.WaitID()] = pending
	}
	snapshots := make(map[agent.ProcessID]agent.ProcessSnapshot)
	for _, snapshot := range head.ProcessSnapshots() {
		snapshots[snapshot.Relation().ProcessID()] = snapshot
	}
	prepared := make([]preparedInteractionAnswer, 0, len(orderedAnswers))
	seenWaits := make(map[agent.WaitID]bool, len(orderedAnswers))
	for _, answer := range orderedAnswers {
		waitID, err := agent.ParseWaitID(answer.RequestID)
		if err != nil {
			return nil, fmt.Errorf("%w: parse answered Interaction input: %w", runs.ErrExecutorStateLost, err)
		}
		if seenWaits[waitID] {
			return nil, fmt.Errorf("%w: answer set repeats a wait", runs.ErrExecutorStateLost)
		}
		seenWaits[waitID] = true
		// The answer addresses a wait, not a member. The Tool child holding that
		// wait is the staged cut's own fact, so the member never has to name it.
		pending, found := targets[waitID]
		if !found || pending.memberID.String() != answer.MemberID {
			return nil, fmt.Errorf("%w: interrupt answer does not address the accepted Interaction input", runs.ErrExecutorStateLost)
		}
		parent, child := snapshots[pending.processID].Relation().ParentID()
		if !child || parent != pending.memberID {
			return nil, fmt.Errorf("%w: accepted input recipient changed its member", runs.ErrExecutorStateLost)
		}
		process, found := i.engine.Process(pending.processID)
		if !found {
			return nil, fmt.Errorf("%w: answered Interaction input is unavailable", runs.ErrExecutorStateLost)
		}
		response, err := runinput.EncodeResolution(answer.Resolution)
		if err != nil {
			return nil, err
		}
		signalID, err := interactionAnswerSignalID(answer, response)
		if err != nil {
			return nil, err
		}
		signal, err := interaction.NewToolInputResponseSignal(signalID, waitID, response)
		if err != nil {
			return nil, fmt.Errorf("execution: construct Interaction answer Signal: %w", err)
		}
		admitted, err := exactAnswerReceipt(snapshots[pending.processID], signal)
		if err != nil {
			return nil, err
		}
		if !admitted {
			current, found := currentByWait[waitID]
			if !found || current.ProcessID() != pending.processID {
				return nil, fmt.Errorf("%w: accepted answer has neither an exact receipt nor its wait", runs.ErrExecutorStateLost)
			}
			signal, err = current.ResponseSignal(signalID, response)
			if err != nil {
				return nil, err
			}
		}
		prepared = append(prepared, preparedInteractionAnswer{process: process, signal: signal, admitted: admitted})
	}
	return prepared, nil
}

func (i *interactionSession) deliverContinuationAnswers(
	ctx context.Context,
	answers []preparedInteractionAnswer,
	input *pendingInteractionContinuation,
) error {
	deliveryContext := runExecutionContext(ctx, i.scope, i.start)
	inputRetained := false
	if input != nil {
		i.state.mu.Lock()
		if i.state.pendingContinuation != nil {
			i.state.mu.Unlock()
			return errors.New("execution: committed continuation input is already pending")
		}
		// The prepared value already detached its content from the caller, and
		// nothing outside this call retains it, so the pending slot takes it
		// rather than copying the copy.
		i.state.pendingContinuation = input
		i.state.mu.Unlock()
		defer func() {
			if !inputRetained {
				i.state.mu.Lock()
				if pending := i.state.pendingContinuation; pending != nil &&
					pending.processID == input.processID && pending.itemID == input.itemID {
					i.state.pendingContinuation = nil
				}
				i.state.mu.Unlock()
			}
		}()
	}
	for _, answer := range answers {
		if answer.admitted {
			continue
		}
		// Scope admits an exact duplicate answer idempotently: false with a nil error
		// means this Signal's immutable content was already accepted, which a
		// partial-delivery restart reconciles as success rather than failing. A
		// conflicting identity or payload returns an error, handled here.
		if _, err := answer.process.DeliverSignals(deliveryContext, answer.signal); err != nil {
			return fmt.Errorf("execution: deliver Interaction answer Signal: %w", err)
		}
	}
	inputRetained = input != nil
	return nil
}

func exactAnswerReceipt(snapshot agent.ProcessSnapshot, signal agent.SignalRequest) (bool, error) {
	for _, receipt := range snapshot.SignalReceipts() {
		if receipt.ID() != signal.ID() {
			continue
		}
		if !receipt.Matches(signal) {
			return false, fmt.Errorf("%w: accepted answer Signal identity conflicts with its receipt", runs.ErrExecutorStateLost)
		}
		if receipt.Consumed() {
			return false, fmt.Errorf("%w: accepted answer was consumed", runs.ErrExecutorStateLost)
		}
		return true, nil
	}
	return false, nil
}

func (i *interactionSession) proveContinuationDelivery(ctx context.Context, answers []preparedInteractionAnswer) error {
	root := i.state.processHandle()
	if root == nil {
		return runs.ErrExecutorNotLive
	}
	head, err := i.committedTree(ctx, root.Relation().ProcessID())
	if err != nil {
		return err
	}
	i.state.mu.Lock()
	checkpoint := i.state.waitingCheckpoint.Clone()
	i.state.mu.Unlock()
	state, err := decodeExecutorCheckpoint(checkpoint)
	if err != nil {
		return err
	}
	if err := validateAcceptedPausedTree(state.handoff, head, i.acceptedRetiredProcesses(state.handoff, head)); err != nil {
		return err
	}
	snapshots := make(map[agent.ProcessID]agent.ProcessSnapshot)
	for _, snapshot := range head.ProcessSnapshots() {
		snapshots[snapshot.Relation().ProcessID()] = snapshot
	}
	for _, answer := range answers {
		matched, err := exactAnswerReceipt(snapshots[answer.process.Relation().ProcessID()], answer.signal)
		if err != nil {
			return err
		}
		if !matched {
			return errors.New("execution: accepted answer has no acknowledged receipt")
		}
	}
	return nil
}

func (i *interactionSession) prepareCommittedContinuationInput(
	input *runs.CommittedUserInput,
) (*pendingInteractionContinuation, error) {
	if input == nil {
		return nil, nil
	}
	if err := resourceid.ValidateItem(input.ItemID); err != nil {
		return nil, fmt.Errorf("execution: committed continuation input: %w", err)
	}
	if _, err := runs.MaterializeUserMessage(input.Content); err != nil {
		return nil, fmt.Errorf("execution: materialize committed continuation input: %w", err)
	}
	process := i.state.processHandle()
	if process == nil {
		return nil, runs.ErrExecutorNotLive
	}
	return &pendingInteractionContinuation{
		processID: process.Relation().ProcessID(), itemID: input.ItemID,
		content: transcript.CloneContent(input.Content),
	}, nil
}

// SubmitSteer queues one user message for the next Interaction safe boundary.
// Agent Framework rejects it while the Process is waiting; accepted content is
// projected immediately before the model request that can first observe it.
func (i *InteractionExecutor) SubmitSteer(
	ctx context.Context,
	ref runs.ExecutorRef,
	input []transcript.ContentBlock,
) (string, error) {
	session, err := i.session(ref)
	if err != nil {
		return "", err
	}
	message, err := runs.MaterializeUserMessage(input)
	if err != nil {
		return "", err
	}
	return session.submitSteer(ctx, message, input)
}

func interactionAnswerSignalID(
	answer runs.InterruptAnswer,
	response jsontext.Value,
) (agent.SignalID, error) {
	digest := sha256.New()
	_, _ = digest.Write([]byte(answer.InterruptItemID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(answer.MemberID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(answer.RequestID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(response)
	return agent.ParseSignalID("answer:" + hex.EncodeToString(digest.Sum(nil)))
}
