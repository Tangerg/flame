package segment

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"

	framehost "github.com/Tangerg/frame/host"
)

func (e *Effects) ReserveChildRunStart(
	ctx context.Context,
	reservation runs.ChildRunStartReservation,
) error {
	record, err := childRunStartReservationRecord(reservation)
	if err != nil {
		return err
	}
	return e.tx(ctx, func(ctx context.Context) error {
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
	changed, err := e.childRunStarts.Conclude(ctx, record, runs.ChildRunStarted)
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

func (e *Effects) AbortChildRunStart(
	ctx context.Context,
	reservation runs.ChildRunStartReservation,
) error {
	record, err := childRunStartReservationRecord(reservation)
	if err != nil {
		return err
	}
	return e.tx(ctx, func(ctx context.Context) error {
		if _, err := e.childRunStarts.Conclude(
			ctx, record, runs.ChildRunStartAborted,
		); err != nil {
			return fmt.Errorf("segment: abort child Run start: %w", err)
		}
		return nil
	})
}

func childRunStartReservationRecord(
	reservation runs.ChildRunStartReservation,
) (persistence.ChildRunStartReservationRecord, error) {
	if err := reservation.Validate(); err != nil {
		return persistence.ChildRunStartReservationRecord{}, fmt.Errorf("segment: invalid child Run start reservation: %w", err)
	}
	payload, err := json.Marshal(childRunStartReservationPayload{
		ExecutorID:      reservation.ExecutorID,
		ParentMemberID:  reservation.Member.ParentID,
		SpawnCallID:     reservation.Member.SpawnCallID,
		ParentRunID:     reservation.Binding.ParentRunID,
		SegmentID:       reservation.SegmentID,
		SpawnedByItemID: reservation.SpawnedByItemID,
		RootRunID:       reservation.RootRunID,
	})
	if err != nil {
		return persistence.ChildRunStartReservationRecord{}, fmt.Errorf("segment: encode child Run start reservation: %w", err)
	}
	return persistence.ChildRunStartReservationRecord{
		MemberID: reservation.Member.MemberID, RunID: reservation.Binding.RunID, SessionID: reservation.SessionID,
		Payload: payload, CreatedAt: reservation.ReservedAt.UTC(),
	}, nil
}

type childRunStartReservationPayload struct {
	ExecutorID      string             `json:"executorId"`
	ParentMemberID  string             `json:"parentMemberId"`
	SpawnCallID     string             `json:"spawnCallId"`
	ParentRunID     identity.RunID     `json:"parentRunId"`
	SegmentID       identity.SegmentID `json:"segmentId"`
	SpawnedByItemID identity.ItemID    `json:"spawnedByItemId"`
	RootRunID       identity.RunID     `json:"rootRunId"`
}

func (e *Effects) CommitOpening(ctx context.Context, opening runs.OpeningCommit) error {
	err := e.tx(ctx, func(ctx context.Context) error {
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
	if acceptance, present := opening.CommandAcceptance(); present {
		if err := e.commandAcceptances.Save(ctx, acceptance); err != nil {
			return fmt.Errorf("segment: record command acceptance: %w", err)
		}
	}
	sessionID, runID, segmentID, err := opening.OwnerIdentity()
	if err != nil {
		return err
	}
	if err := e.runState.RecordRunCommit(ctx, sessionID, runID, segmentID, opening.CommitID()); err != nil {
		return fmt.Errorf("segment: record opening commit receipt: %w", err)
	}
	return nil
}

func (e *Effects) reconcileOpeningCommit(ctx context.Context, opening runs.OpeningCommit) (bool, error) {
	sessionID, runID, segmentID, err := opening.OwnerIdentity()
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
	// The Run row references its Session, so a created Session is inserted first.
	write := opening.SessionWrite()
	if insert, ok := write.(runs.SessionInsert); ok {
		if err := e.sessions.Insert(ctx, insert.Session); err != nil {
			return fmt.Errorf("segment: persist opening initial Session: %w", err)
		}
	}
	if err := e.runState.Admit(ctx, admission); err != nil {
		return err
	}
	if save, ok := write.(runs.SessionSave); ok {
		if err := e.sessions.Save(ctx, save.Replacement); err != nil {
			return fmt.Errorf("segment: persist opening Session replacement: %w", err)
		}
	}
	if actor, present := opening.Actor(); present {
		if err := e.actorAuthority.Admit(ctx, actor); err != nil {
			return err
		}
	}
	return nil
}

// A tree interruption must use CommitTreeBarrier so no individual Run can publish
// a partial barrier.
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
	err := e.runCheckpointTx(ctx, commit.CommitID.String(), commit.ExecutionContext, func(ctx context.Context) error {
		if err := e.applyCommit(ctx, commit); err != nil {
			return err
		}
		if commit.ExecutionContext != nil {
			if err := commit.ExecutionContext.ValidateOwnership(commit.ExecutionContext.RootMemberID, commit.SessionID); err != nil {
				return err
			}
			if err := e.executorCheckpoints.SaveCheckpoint(ctx, *commit.ExecutionContext); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, framehost.ErrRecoveryRequired) {
		return err
	}
	if err != nil {
		settled, settleErr := e.reconcileEventCommit(ctx, commit)
		if settled {
			e.invalidateCompactedContext(commit)
			return nil
		}
		if settleErr != nil {
			err = errors.Join(err, settleErr)
		}
		return err
	}
	e.invalidateCompactedContext(commit)
	return nil
}

func (e *Effects) invalidateCompactedContext(commit runs.EventCommit) {
	for _, write := range commit.Items {
		if write.Item().Kind() == transcript.Compaction {
			e.contextState.ForgetSessionContext(commit.SessionID)
			return
		}
	}
}

const runCommitReconciliationTimeout = 5 * time.Second

// SQLite can report an error after COMMIT crossed its durable boundary, so the
// exact marker decides the outcome; blindly retrying model/tool transitions is
// unsafe.
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
	sessionID identity.SessionID,
	runID identity.RunID,
	segmentID identity.SegmentID,
	commitID runs.CommitID,
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

func (e *Effects) CommitTreeBarrier(ctx context.Context, barrier runs.TreeBarrierCommit) error {
	pending := barrier.Pending()
	checkpoint := barrier.Checkpoint()
	commits := barrier.Runs()
	commitID := barrier.CommitID()

	err := e.runCheckpointTx(ctx, commitID.String(), &checkpoint, func(ctx context.Context) error {
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
	if errors.Is(err, framehost.ErrRecoveryRequired) {
		return err
	}
	var rootSegmentID identity.SegmentID
	for _, commit := range commits {
		if commit.RunID == pending.RootRunID {
			rootSegmentID = commit.SegmentID
			break
		}
	}
	if rootSegmentID.IsZero() {
		return errors.Join(err, errors.New("segment: tree barrier has no root Run commit"))
	}
	settled, settleErr := e.reconcileRunCommit(
		ctx, barrier.SessionID(), pending.RootRunID, rootSegmentID, commitID,
	)
	if settled {
		return nil
	}
	return errors.Join(err, settleErr)
}

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
	for _, replacement := range acceptance.ItemReplacements() {
		if err := e.itemReplacer.ReplaceItem(ctx, replacement); err != nil {
			return fmt.Errorf("segment: record answered interrupt %q: %w", replacement.Expected().ID(), err)
		}
	}
	for _, rule := range acceptance.RememberedRules() {
		if err := e.rememberedRules.Put(ctx, rule); err != nil {
			return fmt.Errorf("segment: persist remembered approval rule %q: %w", rule.ID(), err)
		}
	}
	resume, _ := opening.Resume()
	inputItemID, err := opening.UserInputItemID()
	if err != nil {
		return fmt.Errorf("segment: derive accepted continuation input: %w", err)
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

func (e *Effects) runCheckpointTx(ctx context.Context, identity string, checkpoint *runs.ExecutorCheckpoint, fn func(context.Context) error) error {
	if checkpoint == nil || len(checkpoint.Bindings) == 0 && len(checkpoint.Resources) == 0 {
		return e.tx(ctx, fn)
	}
	return e.pluginBindings(ctx, identity, checkpoint.Clone(), fn)
}

func (e *Effects) openInterrupt(ctx context.Context, p runs.Pending) error {
	if err := e.interrupts.Open(ctx, p); err != nil {
		return fmt.Errorf("segment: persist interrupt: %w", err)
	}
	return nil
}
