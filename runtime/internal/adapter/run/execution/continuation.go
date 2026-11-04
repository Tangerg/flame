package execution

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"

	scoperun "github.com/Tangerg/scope/agent/run"
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
	process  *scoperun.Handle[scoperun.Payload]
	signal   scoperun.Signal
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
	plans, err := continuationAnswerPlans(checkpointState.handoff, head, retired, answers)
	if err != nil {
		return nil, err
	}
	prepared := make([]preparedInteractionAnswer, 0, len(plans))
	for _, plan := range plans {
		process, found := i.engine.Process(plan.processID)
		if !found {
			return nil, fmt.Errorf("%w: answered Interaction input is unavailable", runs.ErrExecutorStateLost)
		}
		prepared = append(prepared, preparedInteractionAnswer{process: process, signal: plan.signal, admitted: plan.admitted})
	}
	return prepared, nil
}

type interactionAnswerPlan struct {
	processID scoperun.ProcessID
	signal    scoperun.Signal
	admitted  bool
}

// Plans are derived only from acknowledged facts. Constructing Signals here
// validates responses but grants no delivery or writer authority.
func continuationAnswerPlans(cut interactionHandoff, head scoperun.Log, retired map[scoperun.ProcessID]bool, answers []runs.InterruptAnswer) ([]interactionAnswerPlan, error) {
	targets := make(map[string]interactionInputTarget)
	for request, target := range cut.inputs {
		if !retired[target.processID] {
			targets[request] = target
		}
	}
	ordered := slices.Clone(answers)
	slices.SortFunc(ordered, func(a, b runs.InterruptAnswer) int {
		if order := strings.Compare(a.MemberID, b.MemberID); order != 0 {
			return order
		}
		return strings.Compare(a.RequestID, b.RequestID)
	})
	if len(ordered) != len(targets) {
		return nil, fmt.Errorf("%w: answer set does not cover the accepted inputs", runs.ErrExecutorStateLost)
	}
	inspection, err := head.Inspect()
	if err != nil {
		return nil, err
	}
	waits := make(map[inputAddress]scoperun.Wait)
	for _, process := range inspection.Processes {
		if retired[process.ID] {
			continue
		}
		for _, wait := range process.Waits {
			address := inputAddress{process: process.ID, wait: wait.Key}
			target, found := targets[inputRequestID(process.ID, wait.Key)]
			if !found || target.processID != process.ID || target.waitKey != wait.Key {
				return nil, fmt.Errorf("%w: current input differs from its accepted target", runs.ErrExecutorStateLost)
			}
			waits[address] = wait
		}
	}
	plans := make([]interactionAnswerPlan, 0, len(ordered))
	seen := make(map[string]bool)
	for _, answer := range ordered {
		target, found := targets[answer.RequestID]
		if !found || target.memberID.String() != answer.MemberID || seen[answer.RequestID] {
			return nil, fmt.Errorf("%w: answer does not address one accepted input", runs.ErrExecutorStateLost)
		}
		seen[answer.RequestID] = true
		if cut.progress[target.processID].OwnerID != target.memberID.String() {
			return nil, fmt.Errorf("%w: input recipient changed its product owner", runs.ErrExecutorStateLost)
		}
		response, err := runinput.EncodeResolution(answer.Resolution)
		if err != nil {
			return nil, err
		}
		id, err := interactionAnswerSignalID(answer, response)
		if err != nil {
			return nil, err
		}
		payload, err := scoperun.ParsePayload(response)
		if err != nil {
			return nil, err
		}
		signal := scoperun.Signal{ID: id, Wait: target.waitKey, Payload: payload}
		admitted, err := exactSignalAdmission(head, target.processID, signal)
		if err != nil {
			return nil, err
		}
		if !admitted {
			wait, found := waits[inputAddress{process: target.processID, wait: target.waitKey}]
			if !found {
				return nil, fmt.Errorf("%w: accepted answer has neither its receipt nor its wait", runs.ErrExecutorStateLost)
			}
			if err := wait.AnswerSchema.Validate(payload.JSON()); err != nil {
				return nil, err
			}
		}
		plans = append(plans, interactionAnswerPlan{processID: target.processID, signal: signal, admitted: admitted})
	}
	return plans, nil
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

// The paused handoff's committed-step counters prove that an admitted answer
// has not been consumed. The admission record proves its exact immutable input.
func exactSignalAdmission(history scoperun.Log, process scoperun.ProcessID, signal scoperun.Signal) (bool, error) {
	for _, entry := range history.Entries {
		admission, ok := entry.Record.(scoperun.SignalAdmitted)
		if !ok || admission.Process != process || admission.Signal.ID != signal.ID {
			continue
		}
		if !sameSignal(admission.Signal, signal) {
			return false, fmt.Errorf("%w: Signal identity conflicts with committed input", runs.ErrExecutorStateLost)
		}
		return true, nil
	}
	return false, nil
}

func sameSignal(left, right scoperun.Signal) bool {
	return left.ID == right.ID && left.Wait == right.Wait && bytes.Equal(left.Payload.JSON(), right.Payload.JSON())
}

func (i *interactionSession) proveContinuationDelivery(ctx context.Context, answers []preparedInteractionAnswer) error {
	root := i.state.processHandle()
	if root == nil {
		return runs.ErrExecutorNotLive
	}
	head, err := i.committedTree(ctx, root.ID())
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
	for _, answer := range answers {
		matched, err := exactSignalAdmission(head, answer.process.ID(), answer.signal)
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
	if err := identity.ValidateItem(input.ItemID); err != nil {
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
		processID: process.ID(), itemID: input.ItemID,
		content: transcript.CloneContent(input.Content),
	}, nil
}

// SubmitSteer queues one user message for the next Interaction safe boundary.
// Agent Framework rejects it while the Process is waiting; accepted content is
// projected immediately before the model request that can first observe it.
func (i *InteractionExecutor) SubmitSteer(
	ctx context.Context,
	ref runs.ExecutorRef,
	prepared runs.PreparedSteer,
) (string, error) {
	session, err := i.session(ref)
	if err != nil {
		return "", err
	}
	prepared = prepared.Clone()
	message, err := runs.MaterializeUserMessage(prepared.Input)
	if err != nil {
		return "", err
	}
	return session.submitSteer(ctx, message, prepared)
}

func interactionAnswerSignalID(
	answer runs.InterruptAnswer,
	response jsontext.Value,
) (scoperun.SignalID, error) {
	digest := sha256.New()
	_, _ = digest.Write([]byte(answer.InterruptItemID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(answer.MemberID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(answer.RequestID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(response)
	return scoperun.ParseSignalID("answer:" + hex.EncodeToString(digest.Sum(nil)))
}
