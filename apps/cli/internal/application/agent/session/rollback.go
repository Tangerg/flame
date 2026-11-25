package session

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/retry"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type rollbackRuntime interface {
	RollbackSession(context.Context, conversation.RollbackSession) (conversation.RollbackResult, error)
	GetSession(context.Context, string) (conversation.SessionSnapshot, error)
}

type RollbackPreview struct {
	request      conversation.RollbackSession
	droppedCount int
}

func PreviewRollback(snapshot conversation.SessionSnapshot, target conversation.RollbackTarget) (RollbackPreview, error) {
	if err := snapshot.Validate(); err != nil {
		return RollbackPreview{}, fmt.Errorf("preview rollback: %w", err)
	}
	if snapshot.Session.ID != target.SessionID {
		return RollbackPreview{}, errors.New("preview rollback: runtime returned another session")
	}
	last, _ := snapshot.LatestRun()
	request := conversation.RollbackSession{
		RollbackTarget: target, ExpectedLastRootRunID: last.ID,
		ExpectedWorkspacePath: snapshot.Session.Workspace.Path,
	}
	if err := request.Validate(); err != nil {
		return RollbackPreview{}, err
	}
	boundary := -1
	if request.ToRunID != "" {
		boundary = slices.IndexFunc(snapshot.Runs, func(run conversation.Run) bool { return run.ID == request.ToRunID })
		if boundary < 0 {
			return RollbackPreview{}, fmt.Errorf("%w: %s", conversation.ErrRunNotFound, request.ToRunID)
		}
		if !snapshot.Runs[boundary].Lineage.IsRoot() {
			return RollbackPreview{}, fmt.Errorf("rollback run %s is not a root run", request.ToRunID)
		}
	}
	count := 0
	if !request.FilesOnly() {
		for i := boundary + 1; i < len(snapshot.Runs); i++ {
			if snapshot.Runs[i].Lineage.IsRoot() {
				count = len(snapshot.Runs) - i
				break
			}
		}
	}
	return RollbackPreview{request: request, droppedCount: count}, nil
}

func (p RollbackPreview) DroppedCount() int { return p.droppedCount }

type RollbackResult struct {
	Outcome  mutation.Outcome
	Snapshot conversation.SessionSnapshot
	Refusal  error
}

func Rollback(
	ctx context.Context,
	runtime rollbackRuntime,
	store *workbench.Store,
	preview RollbackPreview,
	policy mutation.ReplayPolicy,
	backoff retry.Backoff,
) (RollbackResult, error) {
	if store == nil {
		return RollbackResult{}, workbench.ErrUnavailable
	}
	stagedAt := policy.Now()
	replay, err := policy.NewGuardAt(stagedAt)
	if err != nil {
		return RollbackResult{}, err
	}
	request := preview.request
	pending := workbench.PendingSessionRollback{
		CommandID: mutation.NewCommandID(), SessionID: request.SessionID,
		ToRunID: request.ToRunID, Scope: request.Scope,
		ExpectedLastRootRunID: request.ExpectedLastRootRunID,
		ExpectedWorkspacePath: request.ExpectedWorkspacePath,
		StagedAt:              stagedAt, Replay: replay,
	}
	if err := store.StageSessionRollback(pending); err != nil {
		return RollbackResult{}, fmt.Errorf("stage session rollback: %w", err)
	}
	result, err := settleRollback(ctx, runtime, store, pending, policy, backoff)
	if err != nil || result.Outcome != mutation.Confirmed {
		return result, err
	}
	result.Snapshot, err = runtime.GetSession(ctx, pending.SessionID)
	return result, err
}

func settleRollback(
	ctx context.Context,
	runtime rollbackRuntime,
	store *workbench.Store,
	pending workbench.PendingSessionRollback,
	policy mutation.ReplayPolicy,
	backoff retry.Backoff,
) (RollbackResult, error) {
	receipt, err := mutation.ConfirmAdmitted(ctx, backoff, mutation.ReplayAdmission(policy, pending.Replay), func(ctx context.Context) (conversation.RollbackResult, error) {
		if err := store.DispatchReady(); err != nil {
			return conversation.RollbackResult{}, err
		}
		return runtime.RollbackSession(ctx, pending.Request())
	})
	if err != nil {
		if rollbackRefused(err) {
			result := RollbackResult{Outcome: mutation.Rejected, Refusal: err}
			return result, store.RejectSessionRollback(pending.SessionID, pending.CommandID)
		}
		return RollbackResult{Outcome: mutation.Unknown}, fmt.Errorf("confirm original rollback: %w", err)
	}
	if err := receipt.Validate(); err != nil {
		return RollbackResult{Outcome: mutation.Unknown}, fmt.Errorf("validate rollback receipt: %w", err)
	}
	if receipt.Session.ID != pending.SessionID || (pending.Request().FilesOnly() && len(receipt.Dropped) != 0) {
		return RollbackResult{Outcome: mutation.Unknown}, errors.New("rollback receipt disagrees with the original command")
	}
	result := RollbackResult{Outcome: mutation.Confirmed}
	if err := store.ConfirmSessionRollback(pending.SessionID, pending.CommandID, rollbackDraft(receipt.Dropped)); err != nil {
		return result, fmt.Errorf("save rollback restoration: %w", err)
	}
	return result, nil
}

func rollbackDraft(dropped []conversation.DroppedRun) workbench.RollbackDraft {
	for _, run := range dropped {
		var text []string
		var images int
		for _, part := range run.Input {
			switch part.Kind {
			case protocol.ContentBlockText:
				text = append(text, part.Text)
			case protocol.ContentBlockImage:
				images++
			}
		}
		if len(text) != 0 || images != 0 {
			return workbench.RollbackDraft{
				DroppedCount: len(dropped), OpeningText: strings.Join(text, "\n\n"), OpeningImages: images,
			}
		}
	}
	return workbench.RollbackDraft{DroppedCount: len(dropped)}
}

func rollbackRefused(err error) bool {
	if failure, ok := errors.AsType[*protocol.Failure](err); ok {
		switch failure.Problem().Type {
		case protocol.ErrRevisionConflict.Error(),
			protocol.ErrCheckpointUnavailable.Error(), protocol.ErrCheckpointConflict.Error(),
			protocol.ErrRunNotFound.Error(), protocol.ErrSessionNotFound.Error(), protocol.ErrSessionBusy.Error():
			return true
		}
		return false
	}
	return errors.Is(err, conversation.ErrRevisionConflict) || errors.Is(err, conversation.ErrRunNotFound) ||
		errors.Is(err, conversation.ErrSessionNotFound) || errors.Is(err, conversation.ErrSessionBusy) ||
		errors.Is(err, protocol.ErrCheckpointUnavailable) || errors.Is(err, protocol.ErrCheckpointConflict)
}

func RecoverRollbacks(
	ctx context.Context,
	runtime rollbackRuntime,
	store *workbench.Store,
	policy mutation.ReplayPolicy,
	backoff retry.Backoff,
) error {
	for _, pending := range store.PendingSessionRollbacks() {
		if pending.Restoration != nil {
			continue
		}
		result, err := settleRollback(ctx, runtime, store, pending, policy, backoff)
		if err != nil {
			return errors.Join(result.Refusal, err)
		}
		if errors.Is(result.Refusal, conversation.ErrSessionNotFound) {
			if err := store.RetireSessionState(pending.SessionID); err != nil {
				return err
			}
		}
	}
	return nil
}
