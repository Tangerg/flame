package segment

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
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

func (e *Effects) applyStartedChildRun(ctx context.Context, started runs.StartedChildRun) error {
	opening := started.Opening()
	record, err := childRunStartReservationRecord(started.Reservation())
	if err != nil {
		return err
	}
	changed, err := e.childRunStarts.Conclude(ctx, record, sqlite.ChildRunStartConclusionStarted)
	if err != nil {
		return fmt.Errorf("segment: conclude started child Run reservation: %w", err)
	}
	if changed {
		return e.commitOpening(ctx, opening)
	}
	settled, err := e.reconcileOpeningCommit(ctx, opening)
	if settled {
		return nil
	}
	return errors.Join(errors.New("segment: child Run start was concluded by another opening write-set"), err)
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

// CommitOpening accepts one segment atomically. A fresh segment admits its Run;
// a continuation consumes the waiting hand-off, settles its answered Items, and
// resumes the existing Runs in this one transaction. The opening transcript
// projections land in that same transaction, so Start cannot acknowledge a
// segment whose durable opening is missing.
func (e *Effects) CommitOpening(ctx context.Context, opening runs.OpeningCommit) error {
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
	_, admitting := opening.Admission()
	resume, resuming := opening.Resume()
	switch {
	case admitting:
		if err := e.admitOpening(ctx, opening); err != nil {
			return err
		}
	case resuming:
		if err := e.acceptResumeAnswers(ctx, opening); err != nil {
			return err
		}
		if err := e.resumeTree(ctx, resume); err != nil {
			return err
		}
	}
	for _, commit := range opening.Events() {
		if err := e.applyCommit(ctx, commit); err != nil {
			return err
		}
	}
	sessionID, runID, segmentID, err := openingCommitOwner(opening)
	if err != nil {
		return err
	}
	if err := e.runState.RecordRunCommit(ctx, sessionID, runID, segmentID, opening.CommitID()); err != nil {
		return fmt.Errorf("segment: record opening commit receipt: %w", err)
	}
	return nil
}

func openingCommitOwner(opening runs.OpeningCommit) (sessionID, runID, segmentID string, err error) {
	if admission, found := opening.Admission(); found {
		return admission.SessionID, admission.RunID, admission.SegmentID, nil
	}
	resume, found := opening.Resume()
	if !found {
		return "", "", "", errors.New("segment: opening has no owner")
	}
	if err := resume.Validate(); err != nil {
		return "", "", "", fmt.Errorf("segment: resumed opening: %w", err)
	}
	root := resume.Runs[len(resume.Runs)-1].State()
	return root.SessionID(), root.ID(), root.ActiveSegmentID(), nil
}

func (e *Effects) reconcileOpeningCommit(ctx context.Context, opening runs.OpeningCommit) (bool, error) {
	sessionID, runID, segmentID, err := openingCommitOwner(opening)
	if err != nil {
		return false, err
	}
	return e.reconcileRunCommit(ctx, sessionID, runID, segmentID, opening.CommitID())
}

func (e *Effects) admitOpening(ctx context.Context, opening runs.OpeningCommit) error {
	admission, found := opening.Admission()
	if !found {
		return errors.New("segment: opening admission is required")
	}
	if initialSession, present := opening.InitialSession(); present {
		if err := e.sessions.Insert(ctx, initialSession); err != nil {
			return fmt.Errorf("segment: persist opening initial Session: %w", err)
		}
	}
	if err := e.runState.Admit(ctx, admission); err != nil {
		return err
	}
	if replacement, present := opening.SessionReplacement(); present {
		if err := e.sessions.Save(ctx, replacement); err != nil {
			return fmt.Errorf("segment: persist opening Session replacement: %w", err)
		}
	}
	if actor, present := opening.Actor(); present {
		if err := e.actorAuthority.Admit(ctx, actor); err != nil {
			return err
		}
	}
	// An externally initiated command binds its acceptance here, in the same
	// transaction that admits the Run it names. The run_opening record's global
	// key constraint commits atomically with the opening.
	if acceptance, present := opening.CommandAcceptance(); present {
		if err := e.commandAcceptances.Save(ctx, acceptance); err != nil {
			return fmt.Errorf("segment: record command acceptance: %w", err)
		}
	}

	return nil
}

// CommitEvent applies one run event's durable parts atomically: the
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
		if err := e.interrupts.Delete(ctx, commit.RunID); err != nil {
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
	if publication := commit.ResultPublication; publication != nil {
		reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCommitReconciliationTimeout)
		defer cancel()
		return e.runState.ResultPublicationCommitted(reconcileCtx, commit.SessionID, commit.RunID, commit.SegmentID, publication.ID, publication.Digest)
	}

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
	pending := barrier.Pending()
	checkpoint := barrier.Checkpoint()
	commits := barrier.Runs()
	commitID := barrier.CommitID()

	err := e.runInTx(ctx, func(ctx context.Context) error {
		if err := e.executorCheckpoints.SaveCheckpoint(ctx, checkpoint); err != nil {
			return fmt.Errorf("segment: persist tree barrier executor checkpoint: %w", err)
		}
		if err := e.acceptedContinuations.Delete(ctx, pending.RootRunID); err != nil {
			return err
		}
		if err := e.openInterrupt(ctx, pending); err != nil {
			return err
		}
		for _, original := range commits {
			commit := original
			if commit.RunID == pending.RootRunID {
				commit.CommitID = commitID
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
	for _, commit := range commits {
		if commit.RunID == pending.RootRunID {
			rootSegmentID = commit.SegmentID
			break
		}
	}
	if rootSegmentID == "" {
		return errors.Join(err, errors.New("segment: tree barrier has no root Run commit"))
	}
	settled, settleErr := e.reconcileRunCommit(
		ctx, barrier.SessionID(), pending.RootRunID, rootSegmentID, commitID,
	)
	if settled {
		return nil
	}
	err = errors.Join(err, settleErr)
	for _, commit := range commits {
		err = e.compensateFailedCommit(ctx, commit, err)
	}
	return err
}

const stagedToolResultCleanupTimeout = 5 * time.Second

// compensateFailedCommit removes only unbound blobs staged by the failed
// event. Cleanup is request-detached because cancellation is one of the failure
// paths; Discard's unbound predicate makes an ambiguous successful commit safe.
func (e *Effects) compensateFailedCommit(ctx context.Context, commit runs.EventCommit, commitErr error) error {
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
	if publication := commit.ResultPublication; publication != nil {
		committed, err := e.runState.ResultPublicationCommitted(ctx, commit.SessionID, commit.RunID, commit.SegmentID, publication.ID, publication.Digest)
		if err != nil {
			return err
		}
		if committed {
			return nil
		}
	}

	if commit.State == runs.StateTerminalize && commit.Run.Lineage().IsRoot() {
		messages, err := e.conversation.Read(ctx, commit.SessionID)
		if err != nil {
			return fmt.Errorf("segment: read terminal conversation: %w", err)
		}
		outcome, _ := commit.Run.Outcome()
		closure, err := runs.TerminalConversation(ctx, e.runState, commit.SessionID, commit.RunID, append(messages, commit.ConversationMessages...), outcome, commit.Run.Detail())
		if err != nil {
			return err
		}
		commit.ConversationMessages = append(commit.ConversationMessages, closure...)
	}
	if publication := commit.ResultPublication; publication != nil {
		if err := e.runState.RecordResultPublication(ctx, commit.SessionID, commit.RunID, commit.SegmentID, publication.ID, publication.Digest, commit.ToolResults); err != nil {
			return err
		}
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
	if commit.ChargesGoal() {
		if err := e.goalRuns.RecordRun(ctx, *commit.Run); err != nil {
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
				invocation.CallID, invocation.StartedAt, invocation.FinishedAt, invocation.FirstOutputLatencyMillis, invocation.Usage,
			)
		case runs.ModelInvocationFailed:
			err = e.modelInvocations.FailModelInvocation(
				ctx, commit.SessionID, commit.RunID, invocation.SegmentID,
				invocation.CallID, invocation.StartedAt, invocation.FinishedAt, invocation.FirstOutputLatencyMillis,
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
	if err := e.runProgress.UpdateProgress(ctx, *commit.Progress); err != nil {
		return fmt.Errorf("segment: update Run progress: %w", err)
	}
	return nil
}

// acceptResumeAnswers consumes the exact open interrupt set and settles its
// answered Items in the opening transaction. The waiting checkpoint is retained:
// it remains the accepted continuation's execution context until a terminal
// write-set or the next barrier replaces it. A continuation opening without an
// answer write-set resumes a tree whose answers were not re-settled here.
func (e *Effects) acceptResumeAnswers(ctx context.Context, opening runs.OpeningCommit) error {
	acceptance, present := opening.ResumeAcceptance()
	if !present {
		return nil
	}
	expected := acceptance.Pending()
	consumed, found, err := e.interrupts.Consume(ctx, expected.RootRunID)
	if err != nil {
		return fmt.Errorf("segment: consume accepted interrupt set: %w", err)
	}
	if !found {
		return runs.ErrInterruptNotOpen
	}
	if !reflect.DeepEqual(consumed, expected) {
		return errors.New("segment: waiting hand-off changed before answer acceptance")
	}
	replacements, err := acceptance.ItemReplacements()
	if err != nil {
		return fmt.Errorf("segment: prepare answered interrupts: %w", err)
	}
	for _, replacement := range replacements {
		if err := e.itemReplacer.ReplaceItem(ctx, replacement); err != nil {
			return fmt.Errorf("segment: record answered interrupt %q: %w", replacement.Expected().ID(), err)
		}
	}
	// The standing rules are derived and authority-checked by the acceptance owner.
	// Persisting them here binds them to the same transaction as the answers and
	// consumed open set, so a rule never survives an acceptance that rolled back.
	for _, rule := range acceptance.RememberedRules() {
		if err := e.rememberedRules.Put(ctx, rule); err != nil {
			return fmt.Errorf("segment: persist remembered approval rule %q: %w", rule.ID, err)
		}
	}
	// The accepted continuation binding commits with the answers it routes, so a
	// crash before delivery completes leaves exactly one durable accepted-paused
	// tree for boot revival. The next durable waiting/terminal boundary retires it;
	// a local Resume acknowledgement alone cannot prove the head left this cut.
	resume, _ := opening.Resume()
	var inputItemID string
	for _, event := range opening.Events() {
		for _, item := range event.Items {
			if item.Kind() == transcript.UserMessage {
				inputItemID = item.ID()
			}
		}
	}
	accepted, err := runs.NewAcceptedContinuation(acceptance.SessionID(), consumed, resume, inputItemID)
	if err != nil {
		return fmt.Errorf("segment: derive accepted continuation: %w", err)
	}
	if err := e.acceptedContinuations.Put(ctx, accepted); err != nil {
		return fmt.Errorf("segment: persist accepted continuation %q: %w", accepted.RootRunID(), err)
	}
	return nil
}

func (e *Effects) resumeTree(ctx context.Context, resume run.TreeResumeDraft) error {
	if err := resume.Validate(); err != nil {
		return fmt.Errorf("segment: invalid tree resume: %w", err)
	}
	for _, run := range resume.Runs {
		if err := e.runState.Resume(ctx, run); err != nil {
			return fmt.Errorf("segment: resume Run %q state: %w", run.State().ID(), err)
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
	return e.transcript.AppendItem(ctx, item)
}

func (e *Effects) applyState(ctx context.Context, commit runs.EventCommit) error {
	if commit.State == runs.StateUnchanged {
		return nil
	}
	// EventCommit.Validate settles which states carry a Run record and what
	// state that record must be in, so both arms below read one it admitted.
	switch commit.State {
	case runs.StateSuspend:
		return e.runState.Suspend(ctx, *commit.Run, commit.SegmentID, commit.CommitID)
	case runs.StateTerminalize:
		run, err := e.finishedRun(ctx, *commit.Run)
		if err != nil {
			return err
		}
		if err := e.runState.TerminalizeEvent(ctx, run, commit.SegmentID, commit.CommitID); err != nil {
			return err
		}
		if run.Lineage().IsRoot() {
			return e.acceptedContinuations.Delete(ctx, run.ID())
		}
		return nil
	default:
		return fmt.Errorf("segment: unknown run state change %q", commit.State)
	}
}

// finishedRun completes the terminal Run record with the two facts the reducer
// cannot know: the conversation watermark, resolved inside the caller's
// transaction so it is consistent with the state it terminalizes (the message log
// is in its terminal post-compaction shape by the time a terminal event arrives),
// and the row's touch time.
func (e *Effects) finishedRun(ctx context.Context, record run.Run) (run.Run, error) {
	if _, known := record.MessageMark().Count(); record.Lineage().IsRoot() && !known {
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
