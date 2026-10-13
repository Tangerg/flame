package runs

import (
	"context"
	"errors"
	"fmt"

	corechat "github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// Resume validates one complete response set, then opens the continuation in one
// acceptance transaction that consumes the waiting hand-off, settles its answered
// Items, and resumes the tree together. The execution context checkpoint is
// retained for that accepted continuation. Staging reconstructs the live/restored
// tree before acceptance without any durable effect; semantic answer submission
// continues behind the Run lifecycle supervisor after the opening commits.
func (c *Coordinator) Resume(ctx context.Context, cmd ResumeCommand) (result StartResult, err error) {
	cmd = cmd.clone()
	addressed, found, err := c.runs.Run(ctx, cmd.RunID)
	if err != nil {
		return StartResult{}, err
	}
	if !found {
		return StartResult{}, ErrInterruptNotOpen
	}
	sessionID := addressed.SessionID()
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
	// Run identity locates admission; the answerable hand-off and its Items are
	// read only while this command excludes competing resume and cancellation.
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

	acceptance, err := NewResumeAcceptance(sessionID, pending, itemsByID, answers)
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare resume acceptance: %w", err)
	}
	rootContinuation, ok := pending.RootContinuation()
	if !ok {
		return StartResult{}, errors.New("runs: pending interrupt set has no root continuation")
	}
	checkpoint, err := c.checkpoints.ReadWaitingCheckpoint(ctx, rootContinuation.MemberID)
	if err != nil {
		return StartResult{}, err
	}
	waiting, err := waitingContinuationFromPending(pending, checkpoint, parkedRuns, sess)
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare waiting continuation: %w", err)
	}
	// Staging reconstructs in-memory executor state and commits nothing durable.
	// The open interrupt set is consumed only by the acceptance opening below, so a
	// failure before that commit leaves the Run recoverable as waiting.
	ref, err := c.continuation.StageContinuation(ctx, waiting)
	if err != nil {
		return StartResult{}, err
	}
	staged := ownStagedExecution(c.releases, ref)
	defer func() {
		if err != nil {
			err = staged.abandon(ctx, err, "unaccepted continuation")
			result = StartResult{}
		}
	}()
	segmentID := c.newSegmentID()
	var committedInput *CommittedUserInput
	if len(cmd.Input) > 0 {
		committedInput = &CommittedUserInput{
			ItemID:  userMessageItemID(segmentID),
			Content: transcript.CloneContent(cmd.Input),
		}
	}
	continuation, err := treeContinuationFromPending(pending, parkedRuns, itemsByID)
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare tree continuation: %w", err)
	}
	continuation.approvalCalls = acceptance.approvalCalls()
	replacements, err := acceptance.ItemReplacements()
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare tree continuation: %w", err)
	}
	for _, replacement := range replacements {
		item := replacement.State()
		continuation.items[item.ID()] = item
	}
	// One acceptance transaction consumes the open set, settles its answered Items,
	// resumes the tree, and opens the continuation Segment. Executor activation is
	// detached and owned by the accepted Run lifecycle.
	events, err := c.openSegment(ctx, segmentSpec{
		RunID:            cmd.RunID,
		SegmentID:        segmentID,
		SessionID:        sessionID,
		WorkspaceCWD:     sess.Workspace().Path(),
		Isolated:         sess.Isolated(),
		ExecutorID:       ref.ExecutorID,
		Input:            cmd.Input,
		Continuation:     continuation,
		ResumeAcceptance: &acceptance,
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
	// The opening committed the acceptance and transferred the staged tree to the
	// Segment lifecycle.
	staged.transfer()
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
