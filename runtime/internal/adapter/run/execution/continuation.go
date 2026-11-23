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

// Called only after the fresh Segment opening commits, so accepting the Signal
// cannot start work ahead of the durable lifecycle boundary.
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
	restore, err := session.beginContinuation()
	if err != nil {
		return err
	}
	release, err := session.prepareContinuationRelease(ctx, answers)
	if err != nil {
		return err
	}
	committedInput, err := captureCommittedResumeInput(input)
	if err != nil {
		return err
	}
	session.segmentClock.start()
	if restore {
		if err := session.recoverInputHandoff(ctx); err != nil {
			return err
		}
	}
	if err := session.deliverContinuationAnswers(ctx, release.answers); err != nil {
		return err
	}
	if err := session.proveContinuationDelivery(ctx, release.answers); err != nil {
		return err
	}
	if len(release.paused) > 0 {
		if err := session.resumePausedProcesses(ctx, release.paused); err != nil {
			return err
		}
	}
	session.releaseContinuation(committedInput)
	if restore {
		session.startWorkers()
	}
	return nil
}

type continuationRelease struct {
	paused  []scoperun.ProcessID
	answers []interactionAnswerPlan
}

func (i *interactionSession) prepareContinuationRelease(
	ctx context.Context,
	answers []runs.InterruptAnswer,
) (continuationRelease, error) {
	i.state.mu.Lock()
	checkpoint := i.state.waitingCheckpoint.Clone()
	i.state.mu.Unlock()
	checkpointState, err := decodeExecutorCheckpoint(checkpoint)
	if err != nil {
		return continuationRelease{}, fmt.Errorf("%w: decode staged Interaction checkpoint: %w", runs.ErrExecutorStateLost, err)
	}
	head, err := i.committedTree(ctx, checkpointState.handoff.rootID)
	if err != nil {
		return continuationRelease{}, err
	}
	retired := i.acceptedRetiredProcesses(checkpointState.handoff, head)
	return planContinuationRelease(checkpointState.handoff, head, retired, answers)
}

type interactionAnswerPlan struct {
	processID scoperun.ProcessID
	signal    scoperun.Signal
	admitted  bool
}

func planContinuationRelease(cut interactionHandoff, head scoperun.Log, retired map[scoperun.ProcessID]bool, answers []runs.InterruptAnswer) (continuationRelease, error) {
	inspection, err := head.Inspect()
	if err != nil {
		return continuationRelease{}, err
	}
	paused, started, err := acceptedReleaseBoundary(cut, inspection, retired)
	if err != nil {
		return continuationRelease{}, err
	}
	signals, err := continuationAnswerPlans(cut, head, inspection, retired, answers, started)
	if err != nil {
		return continuationRelease{}, err
	}
	return continuationRelease{paused: paused, answers: signals}, nil
}

func continuationAnswerPlans(cut interactionHandoff, head scoperun.Log, inspection scoperun.TreeInspection, retired map[scoperun.ProcessID]bool, answers []runs.InterruptAnswer, releaseStarted bool) ([]interactionAnswerPlan, error) {
	targets := make(map[string]interactionInputTarget)
	for request, target := range cut.inputs {
		if !retired[target.processID] {
			targets[request] = target
		}
	}
	if len(answers) != len(targets) {
		return nil, fmt.Errorf("%w: answer set does not cover the accepted inputs", runs.ErrExecutorStateLost)
	}
	waits, err := acceptedWaits(inspection, retired, targets)
	if err != nil {
		return nil, err
	}
	ordered := slices.Clone(answers)
	slices.SortFunc(ordered, func(a, b runs.InterruptAnswer) int {
		if order := strings.Compare(a.MemberID, b.MemberID); order != 0 {
			return order
		}
		return strings.Compare(a.RequestID, b.RequestID)
	})
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
		plan, err := planAnswer(head, waits, target, answer, releaseStarted)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func acceptedWaits(inspection scoperun.TreeInspection, retired map[scoperun.ProcessID]bool, targets map[string]interactionInputTarget) (map[inputAddress]scoperun.Wait, error) {
	waits := make(map[inputAddress]scoperun.Wait)
	for _, process := range inspection.Processes {
		if retired[process.ID] {
			continue
		}
		for _, wait := range process.Waits {
			target, found := targets[inputRequestID(process.ID, wait.Key)]
			if !found || target.processID != process.ID || target.waitKey != wait.Key {
				return nil, fmt.Errorf("%w: current input differs from its accepted target", runs.ErrExecutorStateLost)
			}
			waits[inputAddress{process: process.ID, wait: wait.Key}] = wait
		}
	}
	return waits, nil
}

func planAnswer(head scoperun.Log, waits map[inputAddress]scoperun.Wait, target interactionInputTarget, answer runs.InterruptAnswer, releaseStarted bool) (interactionAnswerPlan, error) {
	response, err := runinput.EncodeResolution(answer.Resolution)
	if err != nil {
		return interactionAnswerPlan{}, err
	}
	id, err := interactionAnswerSignalID(answer, response)
	if err != nil {
		return interactionAnswerPlan{}, err
	}
	payload, err := scoperun.ParsePayload(response)
	if err != nil {
		return interactionAnswerPlan{}, err
	}
	signal := scoperun.Signal{ID: id, Wait: target.waitKey, Payload: payload}
	admitted, err := exactSignalAdmission(head, target.processID, signal)
	if err != nil {
		return interactionAnswerPlan{}, err
	}
	if !admitted {
		if releaseStarted {
			return interactionAnswerPlan{}, fmt.Errorf("%w: release started before every answer was acknowledged", runs.ErrExecutorStateLost)
		}
		wait, found := waits[inputAddress{process: target.processID, wait: target.waitKey}]
		if !found {
			return interactionAnswerPlan{}, fmt.Errorf("%w: accepted answer has neither its receipt nor its wait", runs.ErrExecutorStateLost)
		}
		if err := wait.AnswerSchema.Validate(payload.JSON()); err != nil {
			return interactionAnswerPlan{}, err
		}
	}
	return interactionAnswerPlan{processID: target.processID, signal: signal, admitted: admitted}, nil
}

func (i *interactionSession) deliverContinuationAnswers(
	ctx context.Context,
	answers []interactionAnswerPlan,
) error {
	deliveryContext := runExecutionContext(ctx, i.scope, i.start)
	for _, answer := range answers {
		if answer.admitted {
			continue
		}
		process, found := i.engine.Process(answer.processID)
		if !found {
			return fmt.Errorf("%w: answered Interaction input is unavailable", runs.ErrExecutorStateLost)
		}
		// Scope admits an exact duplicate answer idempotently: false with a nil error
		// means this Signal's immutable content was already accepted, which a
		// partial-delivery restart reconciles as success rather than failing. A
		// conflicting identity or payload returns an error, handled here.
		if _, err := process.DeliverSignals(deliveryContext, answer.signal); err != nil {
			return fmt.Errorf("execution: deliver Interaction answer Signal: %w", err)
		}
	}
	return nil
}

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

func (i *interactionSession) proveContinuationDelivery(ctx context.Context, answers []interactionAnswerPlan) error {
	head, err := i.committedTree(ctx, i.rootID)
	if err != nil {
		return err
	}
	for _, answer := range answers {
		matched, err := exactSignalAdmission(head, answer.processID, answer.signal)
		if err != nil {
			return err
		}
		if !matched {
			return errors.New("execution: accepted answer has no acknowledged receipt")
		}
	}
	return nil
}

func captureCommittedResumeInput(input *runs.CommittedUserInput) (*runs.CommittedUserInput, error) {
	if input == nil {
		return nil, nil
	}
	if err := input.ItemID.Validate(); err != nil {
		return nil, fmt.Errorf("execution: committed continuation input: %w", err)
	}
	if _, err := runs.MaterializeUserMessage(input.Content); err != nil {
		return nil, fmt.Errorf("execution: materialize committed continuation input: %w", err)
	}
	return &runs.CommittedUserInput{
		ItemID: input.ItemID, Content: transcript.CloneContent(input.Content),
	}, nil
}

func (i *InteractionExecutor) SubmitSteer(
	ctx context.Context,
	ref runs.ExecutorRef,
	prepared runs.PreparedSteer,
) (identity.ItemID, error) {
	session, err := i.session(ref)
	if err != nil {
		return identity.ItemID{}, err
	}
	return session.submitSteer(ctx, prepared)
}

func interactionAnswerSignalID(
	answer runs.InterruptAnswer,
	response jsontext.Value,
) (scoperun.SignalID, error) {
	digest := sha256.New()
	_, _ = digest.Write([]byte(answer.InterruptItemID.String()))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(answer.MemberID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(answer.RequestID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(response)
	return scoperun.ParseSignalID("answer:" + hex.EncodeToString(digest.Sum(nil)))
}
