package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

var (
	ErrTaskWorktreeNotFound = errors.New("workspace: task worktree is not registered")
	ErrTaskWorktreeBusy     = errors.New("workspace: a task or its destination is in use")
	ErrTaskWorktreeChanged  = errors.New("workspace: the worktree or destination changed")
	ErrTaskWorktreeInput    = errors.New("workspace: a valid commit or branch is required")
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
	Branch   string
	Head     string
	Digest   string
	Patch    string
}

type taskWorktreeBackend interface {
	Create(context.Context, string, string, string) (TaskWorktree, error)
	List(context.Context) ([]TaskWorktree, error)
	Get(context.Context, string) (TaskWorktree, error)
	Preview(context.Context, TaskWorktree) (WorktreeHandoff, error)
	Apply(context.Context, WorktreeHandoff) error
	Release(context.Context, TaskWorktree) error
}

type worktreeMutationGate interface {
	AcquireWorkingTreeMutation(string) (func(), bool, error)
}

type worktreeSessions interface {
	List(context.Context) ([]session.Session, error)
}

type TaskWorktrees struct {
	scope   *Scope
	backend taskWorktreeBackend
	gate    worktreeMutationGate
	session worktreeSessions
}

func NewTaskWorktrees(scope *Scope, backend taskWorktreeBackend, gate worktreeMutationGate, sessions worktreeSessions) *TaskWorktrees {
	return &TaskWorktrees{scope: scope, backend: backend, gate: gate, session: sessions}
}

func (w *TaskWorktrees) Create(ctx context.Context, source, base, setup string) (TaskWorktree, error) {
	if strings.TrimSpace(base) != base || base == "" || len(base) > 256 || len(setup) > 4096 {
		return TaskWorktree{}, ErrTaskWorktreeInput
	}
	path, err := w.scope.ResolveRoot(source)
	if err != nil {
		return TaskWorktree{}, err
	}
	release, err := w.claim(path)
	if err != nil {
		return TaskWorktree{}, err
	}
	defer release()
	return w.backend.Create(ctx, path, base, setup)
}

func (w *TaskWorktrees) List(ctx context.Context) ([]TaskWorktree, error) {
	return w.backend.List(ctx)
}

func (w *TaskWorktrees) Preview(ctx context.Context, id string) (WorktreeHandoff, error) {
	return w.handoff(ctx, id, "", false)
}

func (w *TaskWorktrees) Handoff(ctx context.Context, id, digest string) (WorktreeHandoff, error) {
	return w.handoff(ctx, id, digest, true)
}

func (w *TaskWorktrees) handoff(ctx context.Context, id, digest string, apply bool) (WorktreeHandoff, error) {
	task, err := w.backend.Get(ctx, id)
	if err != nil {
		return WorktreeHandoff{}, err
	}
	if !task.Available {
		return WorktreeHandoff{}, ErrTaskWorktreeNotFound
	}
	release, err := w.claim(task.Path)
	if err != nil {
		return WorktreeHandoff{}, err
	}
	defer release()
	releaseSource, err := w.claim(task.Source)
	if err != nil {
		return WorktreeHandoff{}, err
	}
	defer releaseSource()
	preview, err := w.backend.Preview(ctx, task)
	if err != nil || !apply {
		return preview, err
	}
	if digest != preview.Digest {
		return WorktreeHandoff{}, ErrTaskWorktreeChanged
	}
	return preview, w.backend.Apply(ctx, preview)
}

func (w *TaskWorktrees) Release(ctx context.Context, id string) error {
	task, err := w.backend.Get(ctx, id)
	if err != nil {
		return err
	}
	if task.Available {
		release, err := w.claim(task.Path)
		if err != nil {
			return err
		}
		defer release()
	}
	sessions, err := w.session.List(ctx)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if relative, err := filepath.Rel(task.Path, session.Workspace().Path()); err == nil && filepath.IsLocal(relative) {
			return ErrTaskWorktreeBusy
		}
	}
	return w.backend.Release(ctx, task)
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
