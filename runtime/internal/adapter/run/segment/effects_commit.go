package segment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

// ReserveChildRunStart durably records one invisible child identity. It does
// not admit a Run or append Transcript content.
func (e *Effects) ReserveChildRunStart(
	ctx context.Context,
	reservation runs.ChildRunStartReservation,
) error {
	record, err := childRunStartReservationRecord(reservation)
	if err != nil {
		return err
	}
	return e.runInTx(ctx, func(ctx context.Context) error {
		if err := e.childRunStarts.Reserve(ctx, record); err != nil {
			return fmt.Errorf("segment: reserve child Run start: %w", err)
		}
		return nil
	})
}

// CommitStartedChildRun atomically consumes the invisible reservation with the
// public Running child Run and its causal parent Item.
func (e *Effects) CommitStartedChildRun(
	ctx context.Context,
	reservation runs.ChildRunStartReservation,
	opening runs.OpeningCommit,
) error {
	record, err := childRunStartReservationRecord(reservation)
	if err != nil {
		return err
	}
	if validateStartedChildOpeningErr := validateStartedChildOpening(reservation, opening); validateStartedChildOpeningErr != nil {
		return validateStartedChildOpeningErr
	}
	alreadyConcluded := false
	err = e.runInTx(ctx, func(ctx context.Context) error {
		changed, concludeErr := e.childRunStarts.Conclude(
			ctx, record, sqlite.ChildRunStartConclusionStarted,
		)
		if concludeErr != nil {
			return fmt.Errorf("segment: conclude started child Run reservation: %w", concludeErr)
		}
		if !changed {
			alreadyConcluded = true
			return nil
		}
		if commitOpeningErr := e.commitOpening(ctx, opening); commitOpeningErr != nil {
			return fmt.Errorf("segment: commit started child Run opening: %w", commitOpeningErr)
		}
		return nil
	})
	if err == nil {
		if !alreadyConcluded {
			return nil
		}
		settled, settleErr := e.reconcileOpeningCommit(ctx, opening)
		if settled {
			return nil
		}
		return errors.Join(
			errors.New("segment: child Run start was concluded by another opening write-set"),
			settleErr,
		)
	}
	settled, settleErr := e.reconcileOpeningCommit(ctx, opening)
	if settled {
		return nil
	}
	return errors.Join(err, settleErr)
}

// AbortChildRunStart consumes an invisible reservation without creating a Run.
func (e *Effects) AbortChildRunStart(
	ctx context.Context,
	reservation runs.ChildRunStartReservation,
) error {
	record, err := childRunStartReservationRecord(reservation)
	if err != nil {
		return err
	}
	return e.runInTx(ctx, func(ctx context.Context) error {
		if _, err := e.childRunStarts.Conclude(
			ctx, record, sqlite.ChildRunStartConclusionAborted,
		); err != nil {
			return fmt.Errorf("segment: abort child Run start: %w", err)
		}
		return nil
	})
}

func childRunStartReservationRecord(
	reservation runs.ChildRunStartReservation,
) (sqlite.ChildRunStartReservationRecord, error) {
	if err := reservation.Validate(); err != nil {
		return sqlite.ChildRunStartReservationRecord{}, fmt.Errorf("segment: invalid child Run start reservation: %w", err)
	}
	payload, err := json.Marshal(childRunStartReservationPayload{
		ExecutorID:      reservation.ExecutorID,
		ParentMemberID:  reservation.Member.ParentID,
		SpawnCallID:     reservation.Member.SpawnCallID,
		RunID:           reservation.Binding.RunID,
		ParentRunID:     reservation.Binding.ParentRunID,
		SegmentID:       reservation.SegmentID,
		SpawnedByItemID: reservation.SpawnedByItemID,
		RootRunID:       reservation.RootRunID,
	})
	if err != nil {
		return sqlite.ChildRunStartReservationRecord{}, fmt.Errorf("segment: encode child Run start reservation: %w", err)
	}
	return sqlite.ChildRunStartReservationRecord{
		MemberID: reservation.Member.MemberID, SessionID: reservation.SessionID,
		Payload: payload, CreatedAt: reservation.ReservedAt.UTC(),
	}, nil
}

// childRunStartReservationPayload is segment's canonical durable comparison
// payload. MemberID, SessionID, and ReservedAt already have dedicated record
// columns; the remaining reservation facts stay adapter-owned instead of making
// an Application struct's Go layout an implicit storage wire.
type childRunStartReservationPayload struct {
	ExecutorID      string `json:"executorId"`
	ParentMemberID  string `json:"parentMemberId"`
	SpawnCallID     string `json:"spawnCallId"`
	RunID           string `json:"runId"`
	ParentRunID     string `json:"parentRunId"`
	SegmentID       string `json:"segmentId"`
	SpawnedByItemID string `json:"spawnedByItemId"`
	RootRunID       string `json:"rootRunId"`
}

type preparedResumeClaim struct {
	claim                runs.ResumeClaimCommit
	root                 runs.Continuation
	questionReplacements []runs.ItemReplacement
	approvalResolutions  []runs.ToolApprovalResolution
}

func validateStartedChildOpening(
	reservation runs.ChildRunStartReservation,
	opening runs.OpeningCommit,
) error {
	if err := opening.Validate(); err != nil {
		return fmt.Errorf("segment: invalid started child opening: %w", err)
	}
	if opening.Admit == nil || opening.Resume != nil || opening.Admit.RunID != reservation.Binding.RunID ||
		opening.Admit.SessionID != reservation.SessionID ||
		opening.Admit.ParentRunID != reservation.Binding.ParentRunID ||
		opening.Admit.RootRunID != reservation.RootRunID ||
		opening.Admit.SpawnedByItemID != reservation.SpawnedByItemID ||
		opening.Admit.SegmentID != reservation.SegmentID {
		return errors.New("segment: started child opening differs from its reservation")
	}
	return nil
}

// ClaimResume is the waiting-answer linearization point. Loading the exact
// checkpoint, consuming the exact Pending value, and deleting that checkpoint
// share one storage transaction. No executor operation occurs in this method.
func (e *Effects) ClaimResume(
	ctx context.Context,
	claim runs.ResumeClaimCommit,
) (runs.ClaimedResume, error) {
	prepared, err := prepareResumeClaim(claim)
	if err != nil {
		return runs.ClaimedResume{}, err
	}
	var checkpoint runs.ExecutorCheckpoint
	err = e.runInTx(ctx, func(ctx context.Context) error {
		return e.applyResumeClaim(ctx, prepared, &checkpoint)
	})
	if err == nil {
		return claimedResumeResult(claim, checkpoint), nil
	}
	return e.reconcileResumeClaim(ctx, claim, checkpoint, err)
}

func prepareResumeClaim(claim runs.ResumeClaimCommit) (preparedResumeClaim, error) {
	if err := claim.Validate(); err != nil {
		return preparedResumeClaim{}, fmt.Errorf("segment: invalid resume claim: %w", err)
	}
	root, ok := claim.Expected.RootContinuation()
	if !ok {
		return preparedResumeClaim{}, errors.New("segment: resume claim has no root continuation")
	}
	questionReplacements, err := claim.QuestionReplacements()
	if err != nil {
		return preparedResumeClaim{}, fmt.Errorf("segment: prepare answered questions: %w", err)
	}
	approvalResolutions, err := claim.ToolApprovalResolutions()
	if err != nil {
		return preparedResumeClaim{}, fmt.Errorf("segment: prepare Tool approval resolutions: %w", err)
	}
	return preparedResumeClaim{
		claim: claim, root: root,
		questionReplacements: questionReplacements,
		approvalResolutions:  approvalResolutions,
	}, nil
}

func (e *Effects) applyResumeClaim(
	ctx context.Context,
	prepared preparedResumeClaim,
	checkpoint *runs.ExecutorCheckpoint,
) error {
	loaded, err := e.loadResumeCheckpoint(ctx, prepared)
	if err != nil {
		return err
	}
	if err := e.consumeResumePending(ctx, prepared.claim); err != nil {
		return err
	}
	for _, resolution := range prepared.approvalResolutions {
		if err := e.resolveToolApproval(ctx, resolution); err != nil {
			return fmt.Errorf("segment: record Tool approval %q: %w", resolution.Identity.ItemID, err)
		}
	}
	for _, replacement := range prepared.questionReplacements {
		if err := e.itemReplacer.ReplaceItem(ctx, replacement.Expected, replacement.Replacement); err != nil {
			return fmt.Errorf("segment: record answered question %q: %w", replacement.Expected.ID(), err)
		}
	}
	if err := e.executorCheckpoints.DeleteCheckpoints(
		ctx, prepared.claim.Expected.SessionID, []string{prepared.root.MemberID},
	); err != nil {
		return fmt.Errorf("segment: invalidate claimed executor checkpoint: %w", err)
	}
	*checkpoint = loaded.Clone()
	if err := e.runState.RecordWaitingRunCommit(
		ctx, prepared.claim.Expected.SessionID, prepared.claim.Expected.RootRunID, prepared.claim.CommitID,
	); err != nil {
		return fmt.Errorf("segment: record resume claim commit receipt: %w", err)
	}
	return nil
}

func (e *Effects) loadResumeCheckpoint(
	ctx context.Context,
	prepared preparedResumeClaim,
) (runs.ExecutorCheckpoint, error) {
	loaded, err := e.executorCheckpoints.LoadCheckpoint(ctx, prepared.root.MemberID)
	if err != nil {
		return runs.ExecutorCheckpoint{}, fmt.Errorf("segment: load claimed executor checkpoint: %w", err)
	}
	if err := loaded.ValidateOwnership(
		prepared.root.MemberID, prepared.claim.Expected.SessionID,
	); err != nil {
		return runs.ExecutorCheckpoint{}, err
	}
	if loaded.ModelSelection != prepared.root.ModelSelection || loaded.Limits != prepared.root.Limits ||
		loaded.Scope.GoalIncarnationID != prepared.claim.Expected.GoalIncarnationID {
		return runs.ExecutorCheckpoint{}, fmt.Errorf(
			"%w: claimed checkpoint policy differs from Pending", runs.ErrInvalidExecutorCheckpoint,
		)
	}
	return loaded, nil
}

func (e *Effects) consumeResumePending(ctx context.Context, claim runs.ResumeClaimCommit) error {
	consumed, found, err := e.resumeClaims.ClaimResume(
		ctx, claim.Expected.SessionID, claim.Expected.RootRunID, claim.Answers, claim.ClaimedAt,
	)
	if err != nil {
		return fmt.Errorf("segment: consume resume Pending: %w", err)
	}
	if !found {
		return runs.ErrInterruptNotOpen
	}
	if !reflect.DeepEqual(consumed, claim.Expected) {
		return errors.New("segment: waiting hand-off changed before answer claim")
	}
	return nil
}

func (e *Effects) reconcileResumeClaim(
	ctx context.Context,
	claim runs.ResumeClaimCommit,
	checkpoint runs.ExecutorCheckpoint,
	commitErr error,
) (runs.ClaimedResume, error) {
	settled, settleErr := e.reconcileRunCommit(
		ctx, claim.Expected.SessionID, claim.Expected.RootRunID, "", claim.CommitID,
	)
	if !settled {
		return runs.ClaimedResume{}, errors.Join(commitErr, settleErr)
	}
	if err := checkpoint.Validate(); err != nil {
		return runs.ClaimedResume{}, errors.Join(
			commitErr,
			errors.New("segment: committed resume claim checkpoint is unavailable to this caller"),
			err,
		)
	}
	return claimedResumeResult(claim, checkpoint), nil
}

func (e *Effects) resolveToolApproval(
	ctx context.Context,
	resolution runs.ToolApprovalResolution,
) error {
	current, found, err := e.toolApprovals.Item(ctx, resolution.Identity.ItemID)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("running ToolCall is absent")
	}
	if current.SessionID() != resolution.Identity.SessionID ||
		current.RunID() != resolution.Identity.RunID ||
		current.ID() != resolution.Identity.ItemID ||
		!current.OccurredAt().Equal(resolution.Identity.OccurredAt) {
		return fmt.Errorf("%w: running ToolCall identity differs from Pending", transcript.ErrIdentityConflict)
	}
	invocation, present := current.ToolInvocation()
	if !present || !reflect.DeepEqual(invocation, resolution.Invocation) {
		return fmt.Errorf("%w: running ToolCall invocation differs from Pending", transcript.ErrIdentityConflict)
	}
	replacement, err := current.ResolveToolApproval(resolution.Decision)
	if err != nil {
		return err
	}
	return e.toolApprovals.ReplaceItem(ctx, current, replacement)
}

func claimedResumeResult(claim runs.ResumeClaimCommit, checkpoint runs.ExecutorCheckpoint) runs.ClaimedResume {
	return runs.ClaimedResume{
		Pending: claim.Expected, Answers: append([]runs.InterruptAnswer(nil), claim.Answers...),
		Checkpoint: checkpoint,
	}
}

// CommitOpening accepts one segment atomically. A fresh segment admits its Run;
// a continuation resumes the existing Runs after a prior ResumeClaim consumed
// the waiting hand-off and invalidated its checkpoint. The
// opening transcript projections land in that same transaction, so Start cannot
// acknowledge a segment whose durable opening is missing.
func (e *Effects) CommitOpening(ctx context.Context, opening runs.OpeningCommit) error {
	if err := opening.Validate(); err != nil {
		return fmt.Errorf("segment: invalid opening: %w", err)
	}
	err := e.runInTx(ctx, func(ctx context.Context) error {
		return e.commitOpening(ctx, opening)
	})
	if err == nil {
		return nil
	}
	settled, settleErr := e.reconcileOpeningCommit(ctx, opening)
	if settled {
		return nil
	}
	return errors.Join(err, settleErr)
}

// commitOpening assumes that the caller has validated opening and owns the
// transaction boundary. Keeping the persistence body separate prevents
// composite commits from depending on reentrant transaction implementations.
func (e *Effects) commitOpening(ctx context.Context, opening runs.OpeningCommit) error {
	switch {
	case opening.Admit != nil:
		if err := e.admitOpening(ctx, opening); err != nil {
			return err
		}
	case opening.Resume != nil:
		if err := e.resumeTree(ctx, *opening.Resume); err != nil {
			return err
		}
	}
	for _, commit := range opening.Events {
		if err := e.applyCommit(ctx, commit); err != nil {
			return err
		}
	}
	sessionID, runID, segmentID, err := openingCommitOwner(opening)
	if err != nil {
		return err
	}
	if err := e.runState.RecordRunCommit(ctx, sessionID, runID, segmentID, opening.CommitID); err != nil {
		return fmt.Errorf("segment: record opening commit receipt: %w", err)
	}
	return nil
}

func openingCommitOwner(opening runs.OpeningCommit) (sessionID, runID, segmentID string, err error) {
	if opening.Admit != nil {
		return opening.Admit.SessionID, opening.Admit.RunID, opening.Admit.SegmentID, nil
	}
	if opening.Resume == nil {
		return "", "", "", errors.New("segment: opening has no owner")
	}
	for _, resumed := range opening.Resume.Runs {
		if resumed.RunID == opening.Resume.RootRunID {
			return opening.Resume.SessionID, resumed.RunID, resumed.SegmentID, nil
		}
	}
	return "", "", "", errors.New("segment: resumed opening has no root Run")
}

func (e *Effects) reconcileOpeningCommit(ctx context.Context, opening runs.OpeningCommit) (bool, error) {
	sessionID, runID, segmentID, err := openingCommitOwner(opening)
	if err != nil {
		return false, err
	}
	return e.reconcileRunCommit(ctx, sessionID, runID, segmentID, opening.CommitID)
}

func (e *Effects) admitOpening(ctx context.Context, opening runs.OpeningCommit) error {
	if opening.Admit == nil {
		return errors.New("segment: opening admission is required")
	}
	if opening.InitialSession != nil {
		if err := e.sessions.Insert(ctx, *opening.InitialSession); err != nil {
			return fmt.Errorf("segment: persist opening initial Session: %w", err)
		}
	}
	if err := e.runState.Admit(ctx, *opening.Admit); err != nil {
		return err
	}
	if opening.SessionReplacement != nil {
		if err := e.sessions.Save(
			ctx, opening.SessionReplacement.ExpectedRevision,
			opening.SessionReplacement.State,
		); err != nil {
			return fmt.Errorf("segment: persist opening Session replacement: %w", err)
		}
	}
	if opening.ScheduleFiring == "" && opening.ManualScheduleRun == nil {
		return nil
	}
	if e.schedules == nil {
		return errors.New("segment: schedule persistence is unavailable")
	}
	if opening.ManualScheduleRun != nil {
		if err := e.schedules.RecordRun(ctx, *opening.ManualScheduleRun); err != nil {
			return fmt.Errorf("segment: record manual schedule Run: %w", err)
		}
		return nil
	}
	acceptance, err := schedule.NewAcceptance(opening.ScheduleFiring, opening.Admit.RunID)
	if err != nil {
		return fmt.Errorf("segment: form scheduled occurrence acceptance: %w", err)
	}
	if err := e.schedules.Accept(ctx, acceptance); err != nil {
		return fmt.Errorf("segment: accept scheduled occurrence: %w", err)
	}
	return nil
}

// CommitEvent applies one run event's durable parts atomically (§8.3/§8.4): the
// transcript item/run projections and the run-state transition in one
// transaction. A tree interruption is deliberately excluded: it must use
// CommitTreeBarrier so no individual Run can publish a partial barrier.
func (e *Effects) CommitEvent(ctx context.Context, commit runs.EventCommit) error {
	if err := commit.Validate(); err != nil {
		return fmt.Errorf("segment: invalid event commit: %w", err)
	}
	if commit.CommitID.IsZero() {
		return errors.New("segment: event commit identity is required")
	}
	if commit.State == runs.StateSuspend {
		return errors.New("segment: per-Run suspend commit is not allowed")
	}
	if commit.ObsoleteCheckpointRootID != "" && commit.State != runs.StateTerminalize {
		return errors.New("segment: executor checkpoint deletion requires a terminal Run commit")
	}
	err := e.runInTx(ctx, func(ctx context.Context) error {
		if err := e.applyCommit(ctx, commit); err != nil {
			return err
		}
		if commit.ObsoleteCheckpointRootID == "" {
			return nil
		}
		if err := e.executorCheckpoints.DeleteCheckpoints(ctx, commit.SessionID, []string{commit.ObsoleteCheckpointRootID}); err != nil {
			return fmt.Errorf("segment: delete terminal executor checkpoint %q: %w", commit.ObsoleteCheckpointRootID, err)
		}
		if err := e.interrupts.Delete(ctx, commit.SessionID, commit.RunID); err != nil {
			return fmt.Errorf("segment: delete terminal interrupt for root Run %q: %w", commit.RunID, err)
		}
		if err := e.childRunStarts.DeleteSession(ctx, commit.SessionID); err != nil {
			return fmt.Errorf("segment: delete terminal child Run start reservations: %w", err)
		}
		return nil
	})
	if err != nil {
		settled, settleErr := e.reconcileEventCommit(ctx, commit)
		if settled {
			return nil
		}
		if settleErr != nil {
			err = errors.Join(err, settleErr)
		}
		return e.compensateFailedCommit(ctx, commit, err)
	}
	return nil
}

const runCommitReconciliationTimeout = 5 * time.Second

// reconcileEventCommit turns this write-set's exact durable marker into its
// successful outcome. SQLite can report an error after COMMIT has crossed its
// durable boundary, and blindly retrying model/tool state transitions is not
// safe. The aggregate and journals alone are not evidence because they may have
// been written by another Segment or immutable write attempt.
func (e *Effects) reconcileEventCommit(
	ctx context.Context,
	commit runs.EventCommit,
) (bool, error) {
	return e.reconcileRunCommit(
		ctx, commit.SessionID, commit.RunID, commit.SegmentID, commit.CommitID,
	)
}

func (e *Effects) reconcileRunCommit(
	ctx context.Context,
	sessionID string,
	runID string,
	segmentID string,
	commitID runtimeidentity.CommitID,
) (bool, error) {
	reconcileCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		runCommitReconciliationTimeout,
	)
	defer cancel()
	settled, err := e.runState.RunCommitCommitted(
		reconcileCtx, sessionID, runID, segmentID, commitID,
	)
	if err != nil {
		return false, fmt.Errorf("segment: reconcile Run commit: %w", err)
	}
	return settled, nil
}

// CommitTreeBarrier atomically records one root-owned pending set and suspends
// every active Run named by it. The caller supplies Runs in protocol publication
// order; persistence preserves that order while the transaction makes it
// invisible until complete.
func (e *Effects) CommitTreeBarrier(ctx context.Context, barrier runs.TreeBarrierCommit) error {
	if err := barrier.Validate(); err != nil {
		return fmt.Errorf("segment: invalid tree barrier: %w", err)
	}

	err := e.runInTx(ctx, func(ctx context.Context) error {
		if err := e.executorCheckpoints.SaveCheckpoint(ctx, barrier.Checkpoint); err != nil {
			return fmt.Errorf("segment: persist tree barrier executor checkpoint: %w", err)
		}
		if err := e.openInterrupt(ctx, barrier.Pending); err != nil {
			return err
		}
		for _, original := range barrier.Runs {
			commit := original
			if commit.RunID == barrier.Pending.RootRunID {
				commit.CommitID = barrier.CommitID
			}
			if err := e.applyCommit(ctx, commit); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		return nil
	}
	rootSegmentID := ""
	for _, commit := range barrier.Runs {
		if commit.RunID == barrier.Pending.RootRunID {
			rootSegmentID = commit.SegmentID
			break
		}
	}
	if rootSegmentID == "" {
		return errors.Join(err, errors.New("segment: tree barrier has no root Run commit"))
	}
	settled, settleErr := e.reconcileRunCommit(
		ctx, barrier.Pending.SessionID, barrier.Pending.RootRunID, rootSegmentID, barrier.CommitID,
	)
	if settled {
		return nil
	}
	err = errors.Join(err, settleErr)
	for _, commit := range barrier.Runs {
		err = e.compensateFailedCommit(ctx, commit, err)
	}
	return err
}

const stagedToolResultCleanupTimeout = 5 * time.Second

// compensateFailedCommit removes only unbound blobs staged by the failed
// event. Cleanup is request-detached because cancellation is one of the failure
// paths; Discard's unbound predicate makes an ambiguous successful commit safe.
func (e *Effects) compensateFailedCommit(ctx context.Context, commit runs.EventCommit, commitErr error) error {
	if e.toolResults == nil {
		return commitErr
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stagedToolResultCleanupTimeout)
	defer cancel()
	var cleanupErrs []error
	for _, item := range commit.Items {
		invocation, present := item.ToolInvocation()
		if !present || invocation.Offload == nil {
			continue
		}
		if err := e.toolResults.Discard(cleanupCtx, item.SessionID(), *invocation.Offload); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("segment: discard staged tool result %q: %w", invocation.Offload.ID, err))
		}
	}
	return errors.Join(commitErr, errors.Join(cleanupErrs...))
}

func (e *Effects) applyCommit(ctx context.Context, commit runs.EventCommit) error {
	if err := e.runState.RequireActiveSegment(ctx, commit.SessionID, commit.RunID, commit.SegmentID); err != nil {
		return fmt.Errorf("segment: require active event Segment: %w", err)
	}
	for _, item := range commit.Items {
		if err := e.appendItem(ctx, item); err != nil {
			return err
		}
	}
	if len(commit.ConversationMessages) != 0 {
		if err := e.conversation.Write(ctx, commit.SessionID, commit.ConversationMessages...); err != nil {
			return fmt.Errorf("segment: append conversation messages: %w", err)
		}
	}
	if err := e.applyModelInvocations(ctx, commit); err != nil {
		return err
	}
	if err := e.applyToolInvocations(ctx, commit); err != nil {
		return err
	}
	if err := e.applyProgress(ctx, commit); err != nil {
		return err
	}
	if err := e.applyState(ctx, commit); err != nil {
		return err
	}
	if commit.GoalRun != nil {
		if e.goalRuns == nil {
			return errors.New("segment: Goal Run persistence is unavailable")
		}
		if err := e.goalRuns.RecordRun(ctx, *commit.GoalRun); err != nil {
			return fmt.Errorf("segment: record Goal Run: %w", err)
		}
	}
	if commit.State == runs.StateUnchanged && !commit.CommitID.IsZero() {
		if err := e.runState.RecordRunCommit(
			ctx, commit.SessionID, commit.RunID, commit.SegmentID, commit.CommitID,
		); err != nil {
			return fmt.Errorf("segment: record event commit receipt: %w", err)
		}
	}
	return nil
}

func (e *Effects) applyModelInvocations(ctx context.Context, commit runs.EventCommit) error {
	if len(commit.ModelInvocations) == 0 {
		return nil
	}
	for _, invocation := range commit.ModelInvocations {
		var err error
		switch invocation.State {
		case runs.ModelInvocationStarted:
			err = e.modelInvocations.StartModelInvocation(
				ctx, commit.SessionID, commit.RunID, invocation.SegmentID,
				invocation.CallID, invocation.StartedAt,
			)
		case runs.ModelInvocationCompleted:
			err = e.modelInvocations.CompleteModelInvocation(
				ctx, commit.SessionID, commit.RunID, invocation.SegmentID,
				invocation.CallID, invocation.StartedAt, invocation.FinishedAt,
			)
		case runs.ModelInvocationFailed:
			err = e.modelInvocations.FailModelInvocation(
				ctx, commit.SessionID, commit.RunID, invocation.SegmentID,
				invocation.CallID, invocation.StartedAt, invocation.FinishedAt,
			)
		case runs.ModelInvocationUnknown:
			err = e.modelInvocations.MarkModelInvocationUnknown(
				ctx, commit.SessionID, commit.RunID, invocation.SegmentID,
				invocation.CallID, invocation.StartedAt, invocation.FinishedAt,
			)
		default:
			err = fmt.Errorf("unsupported state %q", invocation.State)
		}
		if err != nil {
			return fmt.Errorf("segment: record model invocation %q: %w", invocation.CallID, err)
		}
	}
	return nil
}

func (e *Effects) applyToolInvocations(ctx context.Context, commit runs.EventCommit) error {
	if len(commit.ToolInvocations) == 0 {
		return nil
	}
	for _, invocation := range commit.ToolInvocations {
		var err error
		switch invocation.State {
		case runs.ToolInvocationStarted:
			err = e.toolInvocations.StartToolInvocation(
				ctx, commit.SessionID, commit.RunID, invocation.SegmentID,
				invocation.CallID, invocation.ItemID, invocation.StartedAt,
			)
		case runs.ToolInvocationCompleted:
			err = e.toolInvocations.CompleteToolInvocation(
				ctx, commit.SessionID, commit.RunID, invocation.SegmentID,
				invocation.CallID, invocation.ItemID,
				invocation.StartedAt, invocation.FinishedAt,
			)
		case runs.ToolInvocationIncomplete:
			err = e.toolInvocations.MarkToolInvocationIncomplete(
				ctx, commit.SessionID, commit.RunID, invocation.SegmentID,
				invocation.CallID, invocation.ItemID,
				invocation.StartedAt, invocation.FinishedAt,
			)
		default:
			err = fmt.Errorf("unsupported state %q", invocation.State)
		}
		if err != nil {
			return fmt.Errorf("segment: record Tool invocation %q: %w", invocation.CallID, err)
		}
	}
	return nil
}

func (e *Effects) applyProgress(ctx context.Context, commit runs.EventCommit) error {
	if commit.Progress == nil {
		return nil
	}
	if err := e.runProgress.UpdateProgress(
		ctx,
		commit.SessionID,
		commit.RunID,
		commit.Progress.SegmentID,
		commit.Progress.Metrics,
		commit.Progress.ContextTokens,
		commit.Progress.UpdatedAt,
	); err != nil {
		return fmt.Errorf("segment: update Run progress: %w", err)
	}
	return nil
}

func (e *Effects) resumeTree(ctx context.Context, resume run.TreeResumeDraft) error {
	if err := resume.Validate(); err != nil {
		return fmt.Errorf("segment: invalid tree resume: %w", err)
	}
	if err := e.resumeClaims.RequireResumeClaim(ctx, resume.SessionID, resume.RootRunID); err != nil {
		return fmt.Errorf("segment: require accepted answer claim: %w", err)
	}
	for _, run := range resume.Runs {
		if err := e.runState.Resume(ctx, resume.SessionID, run, resume.ResumedAt); err != nil {
			return fmt.Errorf("segment: resume Run %q state: %w", run.RunID, err)
		}
	}
	return nil
}

func (e *Effects) runInTx(ctx context.Context, fn func(context.Context) error) error {
	return e.tx(ctx, fn)
}

func (e *Effects) openInterrupt(ctx context.Context, p runs.Pending) error {
	if err := e.interrupts.Open(ctx, p); err != nil {
		return fmt.Errorf("segment: persist interrupt: %w", err)
	}
	return nil
}

func (e *Effects) appendItem(ctx context.Context, item transcript.Item) error {
	if err := e.transcript.AppendItem(ctx, item); err != nil {
		return err
	}
	invocation, present := item.ToolInvocation()
	if !present || invocation.Offload == nil {
		return nil
	}
	if invocation.Result == nil {
		return errors.New("segment: offloaded tool result is absent")
	}
	preview, ok := invocation.Result.String()
	if !ok {
		return errors.New("segment: offloaded tool result has no preview string")
	}
	if e.toolResults == nil {
		return errors.New("segment: tool-result persistence is unavailable")
	}
	if err := e.toolResults.Bind(ctx, item.SessionID(), item.ID(), preview, *invocation.Offload); err != nil {
		return fmt.Errorf("segment: bind offloaded tool result: %w", err)
	}
	return nil
}

func (e *Effects) applyState(ctx context.Context, commit runs.EventCommit) error {
	if commit.State == runs.StateUnchanged {
		return nil
	}
	switch commit.State {
	case runs.StateSuspend:
		if commit.Run == nil {
			return errors.New("segment: park commit carries no run record")
		}
		if !commit.CommitID.IsZero() {
			return e.runState.SuspendBarrier(ctx, *commit.Run, commit.SegmentID, commit.CommitID)
		}
		return e.runState.Suspend(ctx, *commit.Run)
	case runs.StateTerminalize:
		run, err := e.finishedRun(ctx, commit)
		if err != nil {
			return err
		}
		return e.runState.TerminalizeEvent(ctx, run, commit.SegmentID, commit.CommitID)
	default:
		return fmt.Errorf("segment: unknown run state change %q", commit.State)
	}
}

// finishedRun completes the terminal Run record with the two facts the reducer
// cannot know: the conversation watermark, resolved inside the caller's
// transaction so it is consistent with the state it terminalizes (the message log
// is in its terminal post-compaction shape by the time a terminal event arrives),
// and the row's touch time.
func (e *Effects) finishedRun(ctx context.Context, commit runs.EventCommit) (run.Run, error) {
	if commit.Run == nil {
		return run.Run{}, errors.New("segment: terminal commit carries no run record")
	}
	record := *commit.Run
	if record.MessageMark() < 0 {
		mark, err := e.conversation.Count(ctx, record.SessionID())
		if err != nil {
			return run.Run{}, fmt.Errorf("segment: resolve terminal message watermark: %w", err)
		}
		finalized, err := record.WithMessageMark(mark)
		if err != nil {
			return run.Run{}, fmt.Errorf("segment: apply terminal message watermark: %w", err)
		}
		record = finalized
	}
	return record, nil
}
