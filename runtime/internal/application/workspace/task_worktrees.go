package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

var (
	ErrTaskWorktreeNotFound = errors.New("workspace: task worktree is not registered")
	ErrTaskWorktreeBusy     = errors.New("workspace: a task or its destination is in use")
	ErrTaskWorktreeChanged  = errors.New("workspace: the worktree or destination changed")
	ErrTaskWorktreeInput    = errors.New("workspace: a valid commit or branch is required")
)

const (
	maxTaskBaseBytes  = 256
	maxTaskSetupBytes = 4096
)

type TaskWorktree struct {
	ID        string
	Path      string
	Source    string
	Base      string
	Setup     *TaskWorktreeSetup
	Available bool
}

type TaskWorktreeSetup struct {
	Command  string
	ExitCode *int
	Output   string
}

type WorktreeHandoff struct {
	Worktree TaskWorktree
	Branch   *string
	Head     string
	Digest   string
	Patch    string
}

type taskWorktreeBackend interface {
	Create(context.Context, string, string, string, operation.Reply[TaskWorktree]) error
	List(context.Context) ([]TaskWorktree, error)
	Get(context.Context, string) (TaskWorktree, error)
	Preview(context.Context, TaskWorktree) (WorktreeHandoff, error)
	Apply(context.Context, WorktreeHandoff) error
	Release(context.Context, TaskWorktree, operation.Reply[struct{}]) error
}

type worktreeMutationGate interface {
	AcquireWorkingTreeMutation(string) (func(), bool, error)
	AcquireWorkingTreeRemoval(string) (func(), bool, error)
}

type worktreeSessions interface {
	List(context.Context) ([]session.Session, error)
}

type TaskWorktrees struct {
	scope         *Scope
	backend       taskWorktreeBackend
	gate          worktreeMutationGate
	session       worktreeSessions
	invalidations invalidation.Publish
}

func NewTaskWorktrees(scope *Scope, backend taskWorktreeBackend, gate worktreeMutationGate, sessions worktreeSessions, invalidations invalidation.Publish) *TaskWorktrees {
	return &TaskWorktrees{scope: scope, backend: backend, gate: gate, session: sessions, invalidations: invalidations}
}

func (w *TaskWorktrees) Create(ctx context.Context, source, base, setup string, complete operation.Reply[TaskWorktree]) error {
	if strings.TrimSpace(base) != base || base == "" || len(base) > maxTaskBaseBytes || len(setup) > maxTaskSetupBytes {
		return ErrTaskWorktreeInput
	}
	path, err := w.scope.ResolveRoot(source)
	if err != nil {
		return err
	}
	return w.publishCatalogChange(w.backend.Create(ctx, path, base, setup, complete))
}

func (w *TaskWorktrees) List(ctx context.Context) ([]TaskWorktree, error) {
	return w.backend.List(ctx)
}

func (w *TaskWorktrees) Preview(ctx context.Context, id string) (WorktreeHandoff, error) {
	task, err := w.availableTask(ctx, id)
	if err != nil {
		return WorktreeHandoff{}, err
	}
	return w.backend.Preview(ctx, task)
}

func (w *TaskWorktrees) ApplyHandoff(ctx context.Context, id, digest string, complete operation.Reply[WorktreeHandoff]) error {
	task, err := w.availableTask(ctx, id)
	if err != nil {
		return err
	}
	release, err := w.claim(task.Path)
	if err != nil {
		return err
	}
	defer release()
	releaseSource, err := w.claim(task.Source)
	if err != nil {
		return err
	}
	defer releaseSource()
	preview, err := w.backend.Preview(ctx, task)
	if err != nil {
		return err
	}
	if digest != preview.Digest {
		return ErrTaskWorktreeChanged
	}
	return complete(ctx, func(work context.Context) (WorktreeHandoff, error) {
		return preview, w.backend.Apply(work, preview)
	})
}

func (w *TaskWorktrees) Release(ctx context.Context, id string, complete operation.Reply[struct{}]) error {
	task, err := w.backend.Get(ctx, id)
	if err != nil {
		return err
	}
	release, ok, err := w.gate.AcquireWorkingTreeRemoval(task.Path)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTaskWorktreeBusy
	}
	defer release()
	sessions, err := w.session.List(ctx)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if relative, err := filepath.Rel(task.Path, session.Workspace().Path()); err == nil && filepath.IsLocal(relative) {
			return ErrTaskWorktreeBusy
		}
	}
	return w.publishCatalogChange(w.backend.Release(ctx, task, complete))
}

func (w *TaskWorktrees) publishCatalogChange(err error) error {
	if err == nil {
		w.invalidations.Notify(invalidation.Notice{Resource: invalidation.TaskWorktrees})
	}
	return err
}

func (w *TaskWorktrees) availableTask(ctx context.Context, id string) (TaskWorktree, error) {
	task, err := w.backend.Get(ctx, id)
	if err != nil {
		return TaskWorktree{}, err
	}
	if !task.Available {
		return TaskWorktree{}, ErrTaskWorktreeNotFound
	}
	return task, nil
}

func (w *TaskWorktrees) claim(path string) (func(), error) {
	release, ok, err := w.gate.AcquireWorkingTreeMutation(path)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrTaskWorktreeBusy
	}
	return release, nil
}
