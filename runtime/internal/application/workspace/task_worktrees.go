package workspace

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
)

var (
	ErrTaskWorktreeNotFound = errors.New("workspace: task worktree is not registered")
	ErrTaskWorktreeBusy     = errors.New("workspace: a task or its destination is in use")
	ErrTaskWorktreeChanged  = errors.New("workspace: the worktree or destination changed")
	ErrTaskWorktreeInput    = errors.New("workspace: a valid commit or branch is required")
	ErrHandoffPatchEncoding = errors.New("workspace: handoff patch contains invalid UTF-8 bytes")
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
	PrepareCreate(context.Context, string, string, string) (func(context.Context) (TaskWorktree, error), error)
	List(context.Context) ([]TaskWorktree, error)
	Get(context.Context, string) (TaskWorktree, error)
	Preview(context.Context, TaskWorktree) (WorktreeHandoff, error)
	Apply(context.Context, WorktreeHandoff) error
	PrepareRelease(context.Context, TaskWorktree) (func(context.Context) error, error)
}

type worktreeMutationGate interface {
	AcquireWorkingTreeMutation(string) (func(), bool, error)
	AcquireWorkingTreeRemoval(string) (func(), bool, error)
}

type TaskWorktrees struct {
	scope         *Scope
	backend       taskWorktreeBackend
	gate          worktreeMutationGate
	sessions      sessionLister
	invalidations invalidation.Publish
}

func NewTaskWorktrees(scope *Scope, backend taskWorktreeBackend, gate worktreeMutationGate, sessions sessionLister, invalidations invalidation.Publish) *TaskWorktrees {
	return &TaskWorktrees{scope: scope, backend: backend, gate: gate, sessions: sessions, invalidations: invalidations}
}

func (w *TaskWorktrees) Create(ctx context.Context, source, base, setup string, complete operation.Reply[TaskWorktree]) error {
	if strings.TrimSpace(base) != base || base == "" || len(base) > maxTaskBaseBytes || len(setup) > maxTaskSetupBytes {
		return ErrTaskWorktreeInput
	}
	path, err := w.scope.ResolveRoot(source)
	if err != nil {
		return err
	}
	create, err := w.backend.PrepareCreate(ctx, path, base, setup)
	if err != nil {
		return err
	}
	return commitTaskCatalogChange(ctx, complete, create, w.invalidations)
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
	sessions, err := w.sessions.List(ctx)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		affected, err := w.scope.paths.RemovalAffectsDirectory(task.Path, session.Workspace().Path())
		if err != nil {
			return err
		}
		if affected {
			return ErrTaskWorktreeBusy
		}
	}
	remove, err := w.backend.PrepareRelease(ctx, task)
	if err != nil {
		return err
	}
	return commitTaskCatalogChange(ctx, complete, func(work context.Context) (struct{}, error) {
		return struct{}{}, remove(work)
	}, w.invalidations)
}

func commitTaskCatalogChange[T any](ctx context.Context, complete operation.Reply[T], write func(context.Context) (T, error), publish invalidation.Publish) error {
	return complete(ctx, func(work context.Context) (T, error) {
		value, err := write(work)
		// A receipt can persist a refusal, so observers follow the catalog write.
		if err == nil {
			publish.Notify(invalidation.Notice{Resource: invalidation.TaskWorktrees})
		}
		return value, err
	})
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
