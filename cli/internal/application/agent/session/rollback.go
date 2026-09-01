package session

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/agent/mutation"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

type rollbackRuntime interface {
	RollbackSession(context.Context, agent.RollbackSession) (agent.RollbackResult, error)
	GetSession(context.Context, string) (agent.SessionSnapshot, error)
}

// RollbackPreview is the exact authoritatively-read before/after projection authorized
// by one rollback confirmation.
type RollbackPreview struct {
	request        agent.RollbackSession
	beforeRevision uint64
	beforeRunIDs   []string
	afterRunIDs    []string
	openingText    string
	openingImages  int
}

// PreviewRollback derives a rollback proof and recoverable opening input from
// one authoritative session snapshot.
func PreviewRollback(snapshot agent.SessionSnapshot, request agent.RollbackSession) (RollbackPreview, error) {
	if err := request.Validate(); err != nil {
		return RollbackPreview{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return RollbackPreview{}, fmt.Errorf("preview rollback: %w", err)
	}
	if snapshot.Session.ID != request.SessionID {
		return RollbackPreview{}, errors.New("preview rollback: runtime returned another session")
	}
	boundary := -1
	if request.ToRunID != "" {
		boundary = slices.IndexFunc(snapshot.Runs, func(run agent.Run) bool { return run.ID == request.ToRunID })
		if boundary < 0 {
			return RollbackPreview{}, fmt.Errorf("%w: %s", agent.ErrRunNotFound, request.ToRunID)
		}
		if !snapshot.Runs[boundary].Lineage.IsRoot() {
			return RollbackPreview{}, fmt.Errorf("rollback run %s is not a root run", request.ToRunID)
		}
	}
	allIDs := make([]string, len(snapshot.Runs))
	for index, run := range snapshot.Runs {
		allIDs[index] = run.ID
	}
	preview := RollbackPreview{
		request: request, beforeRevision: snapshot.Session.Revision,
		beforeRunIDs: slices.Clone(allIDs), afterRunIDs: slices.Clone(allIDs),
	}
	if !request.FilesOnly() {
		dropFrom := 0
		if boundary >= 0 {
			dropFrom = len(snapshot.Runs)
			for index := boundary + 1; index < len(snapshot.Runs); index++ {
				if snapshot.Runs[index].Lineage.IsRoot() {
					dropFrom = index
					break
				}
			}
		}
		preview.afterRunIDs = slices.Clone(allIDs[:dropFrom])
		preview.openingText, preview.openingImages = openingInput(snapshot.Transcript, allIDs[dropFrom:])
	}
	return preview, nil
}

func openingInput(transcript []agent.Block, droppedIDs []string) (string, int) {
	for _, runID := range droppedIDs {
		for _, block := range transcript {
			if block.RunID != runID || block.Kind != agent.BlockUser {
				continue
			}
			images := 0
			for _, attachment := range block.Attachments {
				if attachment.Kind == agent.AttachmentImage {
					images++
				}
			}
			if strings.TrimSpace(block.Text) != "" || images > 0 {
				return block.Text, images
			}
		}
	}
	return "", 0
}

func (p RollbackPreview) Request() agent.RollbackSession { return p.request }

func (p RollbackPreview) DroppedCount() int {
	return len(p.beforeRunIDs) - len(p.afterRunIDs)
}

func (p RollbackPreview) ValidateCommit(snapshot agent.SessionSnapshot) error {
	return validateBefore(p.journal("", commandreplay.UnprotectedGuard(), time.Time{}), snapshot)
}

func (p RollbackPreview) ValidateApplied(snapshot agent.SessionSnapshot) error {
	return validateApplied(p.journal("", commandreplay.UnprotectedGuard(), time.Time{}), snapshot)
}

func (p RollbackPreview) journal(
	commandID agent.CommandID,
	replay commandreplay.Guard,
	stagedAt time.Time,
) workbench.PendingSessionRollback {
	pending := workbench.PendingSessionRollback{
		Phase: workbench.SessionRollbackPrepared, CommandID: commandID,
		SessionID: p.request.SessionID, ToRunID: p.request.ToRunID, Scope: p.request.Scope,
		BeforeRevision: p.beforeRevision, BeforeRunIDs: slices.Clone(p.beforeRunIDs),
		AfterRunIDs: slices.Clone(p.afterRunIDs), OpeningText: p.openingText,
		OpeningImages: p.openingImages, StagedAt: stagedAt, Replay: replay,
	}
	return pending
}

// RollbackResult binds settlement to the exact durable command and authoritative
// session projection.
type RollbackResult struct {
	Pending  workbench.PendingSessionRollback
	Outcome  mutation.Outcome
	Snapshot agent.SessionSnapshot
}

// Rollback verifies an unchanged preview, stages one command identity, then
// settles its runtime outcome without consuming the local recovery record.
func Rollback(
	ctx context.Context,
	runtime rollbackRuntime,
	authoring *workbench.Store,
	preview RollbackPreview,
	policy commandreplay.Policy,
	backoff retry.Backoff,
) (RollbackResult, error) {
	if authoring == nil {
		return RollbackResult{}, errors.New("CLI workbench is unavailable")
	}
	latest, err := runtime.GetSession(ctx, preview.request.SessionID)
	if err != nil {
		return RollbackResult{}, err
	}
	if validateCommitErr := preview.ValidateCommit(latest); validateCommitErr != nil {
		return RollbackResult{}, validateCommitErr
	}
	commandID, err := agent.NewCommandID()
	if err != nil {
		return RollbackResult{}, fmt.Errorf("create session rollback identity: %w", err)
	}
	if err := policy.Validate(); err != nil {
		return RollbackResult{}, err
	}
	stagedAt := policy.Now()
	replay := commandreplay.UnprotectedGuard()
	if preview.request.RestoresFiles() {
		replay, err = policy.NewGuardAt(stagedAt)
		if err != nil {
			return RollbackResult{}, err
		}
	}
	pending := preview.journal(commandID, replay, stagedAt)
	if err := authoring.StageSessionRollback(pending); err != nil {
		return RollbackResult{}, fmt.Errorf("stage session rollback: %w", err)
	}
	return settleRollback(ctx, runtime, pending, policy, backoff, true)
}

// settleRollback observes or replays one prepared command. History projections can
// prove both before and after states. File-affecting commands are replayed only
// while the same runtime idempotency store still guarantees their response.
func settleRollback(
	ctx context.Context,
	runtime rollbackRuntime,
	pending workbench.PendingSessionRollback,
	policy commandreplay.Policy,
	backoff retry.Backoff,
	fresh bool,
) (RollbackResult, error) {
	result := RollbackResult{Pending: pending}
	if err := pending.Validate(); err != nil {
		return result, err
	}
	snapshot, err := runtime.GetSession(ctx, pending.SessionID)
	if err != nil {
		result.Outcome = mutation.Unknown
		return result, fmt.Errorf("read rollback outcome: %w", err)
	}
	if err := validateApplied(pending, snapshot); err == nil {
		result.Outcome, result.Snapshot = mutation.Confirmed, snapshot
		return result, nil
	}
	if err := validateBefore(pending, snapshot); err != nil {
		result.Outcome = mutation.Unknown
		return result, fmt.Errorf("authoritative session matches neither side of the pending rollback: %w", err)
	}
	if pending.Request().RestoresFiles() &&
		!policy.Replayable(pending.Replay) && (!fresh || !policy.CanStart(pending.Replay)) {
		result.Outcome = mutation.Unknown
		return result, errors.New("file rollback replay guarantee expired or belongs to another runtime")
	}

	rollbackResult, rollbackErr := executeRollback(ctx, runtime, pending, policy, backoff, fresh)
	if errors.Is(rollbackErr, agent.ErrCommandStoreMismatch) {
		result.Outcome = mutation.Unknown
		return result, fmt.Errorf("rollback session outcome is unknown: %w", rollbackErr)
	}
	after, readErr := runtime.GetSession(ctx, pending.SessionID)
	if readErr != nil {
		result.Outcome = mutation.Unknown
		var commandErr error
		if rollbackErr != nil {
			commandErr = fmt.Errorf("rollback session: %w", rollbackErr)
		}
		return result, errors.Join(commandErr, fmt.Errorf("read rollback outcome: %w", readErr))
	}
	return reconcileRollback(result, pending, rollbackResult, rollbackErr, after)
}

func executeRollback(
	ctx context.Context,
	runtime rollbackRuntime,
	pending workbench.PendingSessionRollback,
	policy commandreplay.Policy,
	backoff retry.Backoff,
	fresh bool,
) (agent.RollbackResult, error) {
	if pending.Request().HistoryOnly() {
		// History rollback has an authoritative before/after projection. One call
		// followed by another read converges an uncertain acknowledgement without
		// blindly replaying beyond an unrecorded command-store deadline.
		return runtime.RollbackSession(ctx, pending.Request())
	}
	admit := mutation.ReplayAdmission(policy, pending.Replay)
	if fresh {
		admit = mutation.FreshReplayAdmission(policy, pending.Replay)
	}
	return mutation.ConfirmAdmitted(ctx, backoff, admit, func(ctx context.Context) (agent.RollbackResult, error) {
		return runtime.RollbackSession(ctx, pending.Request())
	})
}

func reconcileRollback(
	result RollbackResult,
	pending workbench.PendingSessionRollback,
	rollbackResult agent.RollbackResult,
	rollbackErr error,
	after agent.SessionSnapshot,
) (RollbackResult, error) {
	result.Snapshot = after
	if rollbackErr == nil {
		if err := validateAcknowledged(pending, rollbackResult, after); err != nil {
			result.Outcome = mutation.Unknown
			return result, err
		}
		result.Outcome = mutation.Confirmed
		return result, nil
	}
	if err := validateApplied(pending, after); err == nil {
		result.Outcome = mutation.Confirmed
		return result, rollbackErr
	}
	if pending.Request().HistoryOnly() && !mutation.OutcomeUnknown(rollbackErr) {
		if err := validateBefore(pending, after); err == nil {
			result.Outcome = mutation.Rejected
			return result, rollbackErr
		}
	}
	result.Outcome = mutation.Unknown
	return result, errors.Join(
		fmt.Errorf("rollback session: %w", rollbackErr),
		errors.New("authoritative session does not prove whether the rollback committed"),
	)
}

func validateBefore(pending workbench.PendingSessionRollback, snapshot agent.SessionSnapshot) error {
	if err := validateSnapshot(pending, snapshot); err != nil {
		return err
	}
	if snapshot.Session.Revision != pending.BeforeRevision ||
		!slices.Equal(runIDs(snapshot), pending.BeforeRunIDs) {
		return errors.New("session changed after the rollback preview; review the action again")
	}
	return nil
}

func validateApplied(pending workbench.PendingSessionRollback, snapshot agent.SessionSnapshot) error {
	if pending.Request().FilesOnly() {
		return errors.New("files-only rollback has no authoritative session outcome")
	}
	if err := validateSnapshot(pending, snapshot); err != nil {
		return err
	}
	if len(pending.BeforeRunIDs) == len(pending.AfterRunIDs) ||
		snapshot.Session.Revision <= pending.BeforeRevision ||
		!slices.Equal(runIDs(snapshot), pending.AfterRunIDs) {
		return errors.New("authoritative session does not prove the rollback committed")
	}
	return nil
}

func validateAcknowledged(
	pending workbench.PendingSessionRollback,
	result agent.RollbackResult,
	snapshot agent.SessionSnapshot,
) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if err := validateSnapshot(pending, snapshot); err != nil {
		return err
	}
	if result.Session.ID != pending.SessionID || !slices.Equal(runIDs(snapshot), pending.AfterRunIDs) {
		return errors.New("rollback acknowledgement and authoritative session disagree")
	}
	droppedIDs := make([]string, len(result.Dropped))
	for index, dropped := range result.Dropped {
		droppedIDs[index] = dropped.RunID
	}
	wantDropped := pending.BeforeRunIDs[len(pending.AfterRunIDs):]
	if pending.Request().FilesOnly() {
		wantDropped = nil
	}
	if !slices.Equal(droppedIDs, wantDropped) {
		return errors.New("rollback acknowledgement reports another dropped run set")
	}
	return nil
}

func validateSnapshot(pending workbench.PendingSessionRollback, snapshot agent.SessionSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("read rollback outcome: %w", err)
	}
	if snapshot.Session.ID != pending.SessionID {
		return errors.New("read rollback outcome: runtime returned another session")
	}
	return nil
}

func runIDs(snapshot agent.SessionSnapshot) []string {
	ids := make([]string, len(snapshot.Runs))
	for index, run := range snapshot.Runs {
		ids[index] = run.ID
	}
	return ids
}

// ConfirmRollback upgrades the exact prepared journal after its result reaches the
// caller's presentation boundary.
func ConfirmRollback(authoring *workbench.Store, result RollbackResult) error {
	return authoring.ConfirmSessionRollback(result.Pending.SessionID, result.Pending.CommandID)
}

// RejectRollback retires only the exact prepared journal after a definitive refusal.
func RejectRollback(authoring *workbench.Store, result RollbackResult) error {
	return authoring.RejectSessionRollback(result.Pending.SessionID, result.Pending.CommandID)
}

// RecoverRollbacks settles every prepared rollback before sessions or drafts become
// visible. Confirmed records remain session-owned until activation consumes
// their opening input.
func RecoverRollbacks(
	ctx context.Context,
	runtime rollbackRuntime,
	authoring *workbench.Store,
	policy commandreplay.Policy,
	backoff retry.Backoff,
) error {
	for _, pending := range authoring.PendingSessionRollbacks() {
		if pending.Phase == workbench.SessionRollbackConfirmed {
			continue
		}
		result, err := settleRollback(ctx, runtime, pending, policy, backoff, false)
		switch result.Outcome {
		case mutation.Confirmed:
			if confirmErr := ConfirmRollback(authoring, result); confirmErr != nil {
				return errors.Join(err, confirmErr)
			}
		case mutation.Rejected:
			if rejectErr := RejectRollback(authoring, result); rejectErr != nil {
				return errors.Join(err, rejectErr)
			}
		case mutation.Unknown:
			if errors.Is(err, agent.ErrSessionNotFound) {
				if retireErr := authoring.RetireSessionState(pending.SessionID); retireErr != nil {
					return errors.Join(err, retireErr)
				}
				continue
			}
			return err
		}
	}
	return nil
}
