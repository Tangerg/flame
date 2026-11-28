package delivery

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
)

const (
	TaskWorktreesCreate  Name = "workspaces.tasks.create"
	TaskWorktreesList    Name = "workspaces.tasks.list"
	TaskWorktreesPreview Name = "workspaces.tasks.handoff.preview"
	TaskWorktreesApply   Name = "workspaces.tasks.handoff.apply"
	TaskWorktreesRelease Name = "workspaces.tasks.release"
)

func registerTaskWorktrees(registry *Registry) {
	git := requires(protocol.FeatureGit)
	unavailable, conflict := protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrRevisionConflict.Error()
	readErrors := []string{unavailable, conflict}
	writeErrors := []string{unavailable, protocol.ErrSessionBusy.Error(), conflict}
	registry.command(MethodMeta{Name: TaskWorktreesCreate, GoName: "CreateTaskWorktree", CapabilityRules: git,
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrVcsUnavailable.Error()}},
		func(service interface {
			CreateTaskWorktree(context.Context, protocol.CreateTaskWorktreeRequest) (*protocol.TaskWorktree, error)
		}, ctx context.Context, in protocol.CreateTaskWorktreeRequest) (*protocol.TaskWorktree, error) {
			return service.CreateTaskWorktree(ctx, in)
		}).withOwnedReplay(worktreeReplay((*Handler).CreateTaskWorktree))
	registry.query(MethodMeta{Name: TaskWorktreesList, GoName: "ListTaskWorktrees", CapabilityRules: git},
		func(service interface {
			ListTaskWorktrees(context.Context) (*protocol.Page[protocol.TaskWorktree], error)
		}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.TaskWorktree], error) {
			return service.ListTaskWorktrees(ctx)
		})
	registry.query(MethodMeta{Name: TaskWorktreesPreview, GoName: "PreviewWorktreeHandoff", CapabilityRules: git, Errors: readErrors},
		func(service interface {
			PreviewWorktreeHandoff(context.Context, protocol.TaskWorktreeRequest) (*protocol.WorktreeHandoff, error)
		}, ctx context.Context, in protocol.TaskWorktreeRequest) (*protocol.WorktreeHandoff, error) {
			return service.PreviewWorktreeHandoff(ctx, in)
		})
	registry.command(MethodMeta{Name: TaskWorktreesApply, GoName: "ApplyWorktreeHandoff", CapabilityRules: git, Errors: writeErrors},
		func(service interface {
			ApplyWorktreeHandoff(context.Context, protocol.ApplyWorktreeHandoffRequest) (*protocol.WorktreeHandoff, error)
		}, ctx context.Context, in protocol.ApplyWorktreeHandoffRequest) (*protocol.WorktreeHandoff, error) {
			return service.ApplyWorktreeHandoff(ctx, in)
		}).withOwnedReplay(worktreeReplay((*Handler).ApplyWorktreeHandoff))
	registry.commandAck(MethodMeta{Name: TaskWorktreesRelease, GoName: "ReleaseTaskWorktree", CapabilityRules: git, Errors: writeErrors},
		func(service interface {
			ReleaseTaskWorktree(context.Context, protocol.TaskWorktreeRequest) error
		}, ctx context.Context, in protocol.TaskWorktreeRequest) error {
			return service.ReleaseTaskWorktree(ctx, in)
		}).withOwnedReplay(worktreeReplay(func(s *Handler, ctx context.Context, in protocol.TaskWorktreeRequest) (struct{}, error) {
		return struct{}{}, s.ReleaseTaskWorktree(ctx, in)
	}))
}

func worktreeReplay[P, R any](invoke func(*Handler, context.Context, P) (R, error)) func(*Handler, context.Context, P, keyedCall) protocol.OperationResult {
	return func(s *Handler, ctx context.Context, in P, call keyedCall) protocol.OperationResult {
		payload, err := call.replay.InvokeDeferred(ctx, call.key, call.fingerprint, func(complete *operation.DeferredCompletion) ([]byte, error) {
			if err := complete.ReserveExternal(ctx); err != nil {
				return nil, err
			}
			result, cause := invoke(s, ctx, in)
			var data []byte
			var encodingErr error
			if cause != nil {
				if ctx.Err() != nil {
					return nil, errors.Join(operation.ErrOutcomePending, cause)
				}
				data, encodingErr = encodeStoredOutcome(failed(projectError(cause)))
			} else {
				data, encodingErr = encodeAtomicResult(call.method, result)
			}
			if encodingErr != nil {
				return nil, errors.Join(operation.ErrOutcomePending, encodingErr)
			}
			save, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if err := complete.Stage(save, func(context.Context) ([]byte, error) { return data, nil }); err != nil {
				return nil, errors.Join(operation.ErrOutcomePending, err)
			}
			return data, nil
		}, func(data []byte, _ bool) ([]byte, error) { return data, nil })
		return call.result(ctx, payload, err)
	}
}

func (s *Handler) CreateTaskWorktree(ctx context.Context, in protocol.CreateTaskWorktreeRequest) (*protocol.TaskWorktree, error) {
	task, err := s.taskWorktrees.Create(ctx, in.Source.Path, in.Base, in.Setup)
	if err != nil {
		return nil, taskWorktreeError(err)
	}
	return new(presentTaskWorktree(task)), nil
}

func (s *Handler) ListTaskWorktrees(ctx context.Context) (*protocol.Page[protocol.TaskWorktree], error) {
	tasks, err := s.taskWorktrees.List(ctx)
	if err != nil {
		return nil, taskWorktreeError(err)
	}
	out := make([]protocol.TaskWorktree, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, presentTaskWorktree(task))
	}
	return protocol.NewPage(out), nil
}

func (s *Handler) PreviewWorktreeHandoff(ctx context.Context, in protocol.TaskWorktreeRequest) (*protocol.WorktreeHandoff, error) {
	preview, err := s.taskWorktrees.Preview(ctx, in.ID)
	return presentWorktreeHandoff(preview, err)
}

func (s *Handler) ApplyWorktreeHandoff(ctx context.Context, in protocol.ApplyWorktreeHandoffRequest) (*protocol.WorktreeHandoff, error) {
	preview, err := s.taskWorktrees.ApplyHandoff(ctx, in.ID, in.Digest)
	return presentWorktreeHandoff(preview, err)
}

func (s *Handler) ReleaseTaskWorktree(ctx context.Context, in protocol.TaskWorktreeRequest) error {
	return taskWorktreeError(s.taskWorktrees.Release(ctx, in.ID))
}

func presentWorktreeHandoff(preview workspace.WorktreeHandoff, err error) (*protocol.WorktreeHandoff, error) {
	if err != nil {
		return nil, taskWorktreeError(err)
	}
	return &protocol.WorktreeHandoff{
		Worktree: presentTaskWorktree(preview.Worktree), Branch: preview.Branch, Head: preview.Head,
		Digest: preview.Digest, Patch: strings.ToValidUTF8(preview.Patch, "�"),
	}, nil
}

func presentTaskWorktree(task workspace.TaskWorktree) protocol.TaskWorktree {
	out := protocol.TaskWorktree{
		ID: task.ID, Workspace: protocol.WorkspaceRef{Path: task.Path}, Source: protocol.WorkspaceRef{Path: task.Source},
		Base: task.Base, Available: task.Available,
	}
	if task.Setup != nil {
		out.Setup = &protocol.WorktreeSetup{
			Command: task.Setup.Command, ExitCode: task.Setup.ExitCode, Output: strings.ToValidUTF8(task.Setup.Output, "�"),
		}
	}
	return out
}

func taskWorktreeError(err error) error {
	switch {
	case errors.Is(err, workspace.ErrTaskWorktreeNotFound):
		return protocol.NewFailure(protocol.ErrWorkspaceUnavailable, err.Error(), err)
	case errors.Is(err, workspace.ErrTaskWorktreeBusy):
		return protocol.NewFailure(protocol.ErrSessionBusy, err.Error(), err)
	case errors.Is(err, workspace.ErrTaskWorktreeChanged):
		return protocol.NewFailure(protocol.ErrRevisionConflict, err.Error(), err)
	case errors.Is(err, workspace.ErrTaskWorktreeInput):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	default:
		return wireWorkspaceError(err)
	}
}
