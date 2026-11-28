package sessions

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/google/uuid"
)

type WorktreeForkIntent struct {
	Checkout workspace.WorktreeFork
	Receipt  operation.ReplayClaim
	WriteID  uuid.UUID
}

type ForkPublication struct {
	History HistorySnapshot
	HasPlan bool
}

type worktreeForkJournal interface {
	AdmitWorktreeFork(context.Context, WorktreeForkIntent, ForkPlan) error
	PendingWorktreeFork(context.Context, operation.ReplayClaim) (WorktreeForkIntent, error)
	PendingWorktreeForks(context.Context) ([]WorktreeForkIntent, error)
	PublishWorktreeFork(context.Context, WorktreeForkIntent, []byte, func(session.Session) ([]byte, error)) (*ForkPublication, error)
}

type forkCheckoutWriter interface {
	WriteFork(context.Context, uuid.UUID, workspace.WorktreeFork) error
	ReadForkReceipt(uuid.UUID) (workspace.ForkWriteReceipt, bool, error)
	RetireReceipt(uuid.UUID) error
}

type WorktreeForks struct {
	sessions  *Coordinator
	worktrees *workspace.TaskWorktrees
	journal   worktreeForkJournal
	writer    forkCheckoutWriter
	encode    func(View) ([]byte, error)
}

func NewWorktreeForks(sessions *Coordinator, worktrees *workspace.TaskWorktrees, journal worktreeForkJournal, writer forkCheckoutWriter, encode func(View) ([]byte, error)) *WorktreeForks {
	return &WorktreeForks{sessions: sessions, worktrees: worktrees, journal: journal, writer: writer, encode: encode}
}

func (f *WorktreeForks) Fork(ctx context.Context, spec ForkSpec, approval WorktreeForkApproval, receipt operation.ReplayClaim) error {
	admission, err := f.sessions.ClaimSessionMutation(ctx, spec.ParentID)
	if err != nil {
		return err
	}
	defer admission.Release()
	basis, err := f.sessions.readForkBasis(ctx, spec.ParentID, spec.FromRunID)
	if err != nil {
		return err
	}
	source := basis.source()
	checkout, release, err := f.worktrees.PrepareFork(ctx, source.Path, approval.ID, func(checkout workspace.WorktreeFork) error {
		digest, err := forkReviewDigest(spec, source, checkout)
		if err != nil {
			return err
		}
		if digest != approval.Digest {
			return workspace.ErrTaskWorktreeChanged
		}
		return nil
	})
	if err != nil {
		return err
	}
	defer release()
	plan, err := f.sessions.prepareFork(spec, basis, checkout.Path)
	if err != nil {
		return err
	}
	writerID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	intent := WorktreeForkIntent{Checkout: checkout, Receipt: receipt, WriteID: writerID}
	if err := f.journal.AdmitWorktreeFork(ctx, intent, plan); err != nil {
		return err
	}
	return f.run(context.WithoutCancel(ctx), intent)
}

func (f *WorktreeForks) Resume(ctx context.Context, receipt operation.ReplayClaim) error {
	intent, err := f.journal.PendingWorktreeFork(ctx, receipt)
	if err != nil {
		return err
	}
	return f.run(context.WithoutCancel(ctx), intent)
}

func (f *WorktreeForks) run(ctx context.Context, intent WorktreeForkIntent) error {
	receipt, found, err := f.writer.ReadForkReceipt(intent.WriteID)
	if err != nil {
		return err
	}
	if !found {
		startErr := f.writer.WriteFork(ctx, intent.WriteID, intent.Checkout)
		receipt, found, err = f.writer.ReadForkReceipt(intent.WriteID)
		if !found || err != nil {
			return errors.Join(operation.ErrOutcomePending, err, startErr)
		}
	}
	return f.publish(ctx, intent, receipt)
}

func (f *WorktreeForks) Recover(ctx context.Context) error {
	pending, err := f.journal.PendingWorktreeForks(ctx)
	if err != nil {
		return err
	}
	for _, intent := range pending {
		receipt, found, err := f.writer.ReadForkReceipt(intent.WriteID)
		if errors.Is(err, operation.ErrInProgress) {
			continue
		}
		if err != nil {
			return err
		}
		if found {
			if err := f.publish(ctx, intent, receipt); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *WorktreeForks) publish(ctx context.Context, intent WorktreeForkIntent, native workspace.ForkWriteReceipt) error {
	published, err := f.journal.PublishWorktreeFork(ctx, intent, native.Refusal, func(child session.Session) ([]byte, error) {
		checkout := workspace.DirectoryInspection{RepositoryRoot: child.Workspace().Path()}
		return f.encode(sessionView(child, checkout, activityIdle))
	})
	if err != nil {
		return errors.Join(operation.ErrOutcomePending, err)
	}
	if published != nil {
		f.sessions.publishFork(published.History, published.HasPlan)
		f.sessions.invalidations.Notify(invalidation.Notice{Resource: invalidation.TaskWorktrees})
	}
	if err := f.writer.RetireReceipt(intent.WriteID); err != nil {
		slog.ErrorContext(ctx, "retire published fork checkout receipt", "error", err)
	}
	return nil
}
