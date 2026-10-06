package runs

import (
	"context"
	"errors"
	"fmt"

	corechat "github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// Resume validates one complete response set, atomically consumes the waiting
// hand-off and invalidates its checkpoint, stages the exact live/restored tree,
// then durably opens the continuation Segment. That opening accepts the command;
// semantic answer submission continues behind the Run lifecycle supervisor.
func (c *Coordinator) Resume(ctx context.Context, cmd ResumeCommand) (result StartResult, err error) {
	cmd = cmd.clone()
	pending, found, err := c.interrupts.LookupOpenInterrupt(ctx, cmd.RunID)
	if err != nil {
		return StartResult{}, err
	}
	if !found {
		return StartResult{}, ErrInterruptNotOpen
	}
	// The Items an open interrupt names cannot change while it is open: only
	// the claim below answers them.
	itemsByID, err := c.pendingItems(ctx, pending)
	if err != nil {
		return StartResult{}, err
	}
	interrupts, err := pending.ProjectInterrupts(itemsByID)
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: project open interrupts: %w", err)
	}
	answers, err := resolveResumeResponses(interrupts, pending.Bindings, cmd.Responses)
	if err != nil {
		return StartResult{}, err
	}
	// Reading the Session before the claim is safe only because the open interrupt
	// this resume is answering refuses a relocation for as long as it exists
	// ([sessions.Coordinator.ClaimIdleSession]), and it is consumed under the
	// claim below. Without that, the tree reserved here could be one the Session
	// no longer has.
	sessionID, err := c.pendingSessionID(ctx, pending)
	if err != nil {
		return StartResult{}, err
	}
	sess, err := c.sessionReader.Get(ctx, sessionID)
	if err != nil {
		return StartResult{}, err
	}
	runAdmission, ok, leaseErr := c.admission.AcquireRun(ctx, sessionID, sess.Workspace().Path())
	if leaseErr != nil {
		return StartResult{}, leaseErr
	}
	if !ok {
		return StartResult{}, fmt.Errorf("%w: session %q or working tree %q has a run or mutation in flight", ErrSessionBusy, sessionID, sess.Workspace().Path())
	}
	defer runAdmission.Release()
	parkedRuns, err := c.runs.Tree(ctx, pending.RootRunID)
	if err != nil {
		return StartResult{}, err
	}
	if validatePendingRunTreeErr := validatePendingRunTree(pending, parkedRuns); validatePendingRunTreeErr != nil {
		return StartResult{}, validatePendingRunTreeErr
	}

	root, ok := parkedRoot(pending.RootRunID, parkedRuns)
	if !ok {
		return StartResult{}, errors.New("runs: pending interrupt set has no parked root Run")
	}
	if gap := root.Capabilities().MissingFrom(cmd.CallerCapabilities); !gap.IsEmpty() {
		return StartResult{}, &run.InsufficientCapabilitiesError{RunID: cmd.RunID, Missing: gap}
	}
	if len(cmd.Input) > 0 {
		message, err := MaterializeUserMessage(cmd.Input)
		if err != nil {
			return StartResult{}, err
		}
		if err := c.models.AdmitInput(root.ModelSelection(), []corechat.Message{message}); err != nil {
			return StartResult{}, fmt.Errorf("%w: %w", ErrUnsupportedMedia, err)
		}
	}

	claim, err := NewResumeClaimCommit(newRunCommitID(), sessionID, pending, itemsByID, answers)
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare resume claim: %w", err)
	}
	checkpoint, err := c.resumeClaims.ClaimResume(ctx, claim)
	if err != nil {
		return StartResult{}, err
	}
	attempt := c.ownClaimedResume(sessionID, pending)
	defer func() {
		err = attempt.fail(ctx, err)
		if err != nil {
			result = StartResult{}
		}
	}()
	if validateClaimedCheckpointErr := validateClaimedCheckpoint(checkpoint, pending, sessionID); validateClaimedCheckpointErr != nil {
		return StartResult{}, validateClaimedCheckpointErr
	}
	waiting, err := waitingContinuationFromPending(pending, checkpoint, parkedRuns, sess)
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare waiting continuation: %w", err)
	}
	ref, err := c.continuation.StageContinuation(ctx, waiting)
	if err != nil {
		return StartResult{}, err
	}
	attempt.ownStagedExecution(c.releases, ref)
	segmentID := c.newSegmentID()
	var committedInput *CommittedUserInput
	if len(cmd.Input) > 0 {
		committedInput = &CommittedUserInput{
			ItemID:  userMessageItemID(segmentID),
			Content: transcript.CloneContent(cmd.Input),
		}
	}
	pendingCopy := pending
	continuation, err := treeContinuationFromPending(pendingCopy, parkedRuns, itemsByID)
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare tree continuation: %w", err)
	}
	continuation.approvalVerdicts = claim.approvalVerdicts()
	continuation.answeredQuestions, err = claim.answeredQuestions()
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare tree continuation: %w", err)
	}
	events, err := c.openSegment(ctx, segmentSpec{
		RunID:            cmd.RunID,
		SegmentID:        segmentID,
		SessionID:        sessionID,
		WorkspaceCWD:     sess.Workspace().Path(),
		Isolated:         sess.Isolated(),
		ExecutorID:       ref.ExecutorID,
		Input:            cmd.Input,
		Continuation:     continuation,
		admission:        &runAdmission,
		DetachActivation: true,
		BeginExecution: func(beginCtx context.Context) error {
			return c.continuation.BeginContinuation(
				beginCtx, ref, answers, committedInput,
			)
		},
	})
	if err != nil {
		return StartResult{}, err
	}
	// A successful opening transferred the staged tree to the Segment lifecycle.
	// On failure, the attempt still owns it and compensates durable state first.
	attempt.accept()
	// The continuation is durably accepted, which consumed the whole open set: the
	// run is running again and nothing in this session is waiting on a person.
	for _, continuation := range pending.Continuations {
		if continuation.RunID == pending.RootRunID {
			continue
		}
		c.publications.publishRunMoved(sessionID, continuation.RunID)
	}
	c.publications.publishWaitingMoved(sessionID, pending.RootRunID)
	result = StartResult{RunID: cmd.RunID, SegmentID: segmentID, SessionID: sessionID, Events: events}
	if committedInput != nil {
		// Named only when there is an item to name: the id is derived from the segment
		// the same way a fresh run derives it, so the client reconciles its optimistic
		// bubble by id rather than by content.
		result.UserItemID = committedInput.ItemID
	}
	return result, nil
}

func validateClaimedCheckpoint(checkpoint ExecutorCheckpoint, expected Pending, sessionID string) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	root, ok := expected.RootContinuation()
	if !ok {
		return errors.New("runs: claimed continuation has no root")
	}
	return checkpoint.ValidateOwnership(root.MemberID, sessionID)
}
