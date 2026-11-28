package delivery

import (
	"context"
	"errors"
	"strings"

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
		}).withOwnedReplay(worktreeMutationReplay((*Handler).createTaskWorktree, func(task workspace.TaskWorktree) *protocol.TaskWorktree {
		return new(presentTaskWorktree(task))
	}))
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
		}).withOwnedReplay(worktreeMutationReplay((*Handler).applyWorktreeHandoff, presentWorktreeHandoff))
	registry.commandAck(MethodMeta{Name: TaskWorktreesRelease, GoName: "ReleaseTaskWorktree", CapabilityRules: git, Errors: writeErrors},
		func(service interface {
			ReleaseTaskWorktree(context.Context, protocol.TaskWorktreeRequest) error
		}, ctx context.Context, in protocol.TaskWorktreeRequest) error {
			return service.ReleaseTaskWorktree(ctx, in)
		}).withOwnedReplay(worktreeMutationReplay((*Handler).releaseTaskWorktree, func(value struct{}) struct{} { return value }))
}

func worktreeMutationReplay[P, T, R any](
	invoke func(*Handler, context.Context, P, operation.Reply[T]) error,
	present func(T) R,
) func(*Handler, context.Context, P, keyedCall) protocol.OperationResult {
	return func(s *Handler, ctx context.Context, in P, call keyedCall) protocol.OperationResult {
		refuse := func(cause error) ([]byte, error) {
			return encodeAcknowledgement(taskWorktreeError(cause))
		}
		payload, err := call.replay.InvokeExternal(ctx, call.key, call.fingerprint, func(reply operation.Reply[[]byte]) error {
			return invoke(s, ctx, in, func(ctx context.Context, write func(context.Context) (T, error)) error {
				return reply(ctx, func(work context.Context) ([]byte, error) {
					result, err := write(work)
					if err != nil {
						return refuse(err)
					}
					return encodeAtomicResult(call.method, present(result))
				})
			})
		}, refuse)
		return call.result(ctx, payload, err)
	}
}

func (s *Handler) CreateTaskWorktree(ctx context.Context, in protocol.CreateTaskWorktreeRequest) (*protocol.TaskWorktree, error) {
	var task workspace.TaskWorktree
	if err := s.createTaskWorktree(ctx, in, operation.DirectReply(&task)); err != nil {
		return nil, taskWorktreeError(err)
	}
	return new(presentTaskWorktree(task)), nil
}

func (s *Handler) createTaskWorktree(ctx context.Context, in protocol.CreateTaskWorktreeRequest, complete operation.Reply[workspace.TaskWorktree]) error {
	return s.taskWorktrees.Create(ctx, in.Source.Path, in.Base, in.Setup, complete)
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
	if err != nil {
		return nil, taskWorktreeError(err)
	}
	return presentWorktreeHandoff(preview), nil
}

func (s *Handler) ApplyWorktreeHandoff(ctx context.Context, in protocol.ApplyWorktreeHandoffRequest) (*protocol.WorktreeHandoff, error) {
	var handoff workspace.WorktreeHandoff
	if err := s.applyWorktreeHandoff(ctx, in, operation.DirectReply(&handoff)); err != nil {
		return nil, taskWorktreeError(err)
	}
	return presentWorktreeHandoff(handoff), nil
}

func (s *Handler) applyWorktreeHandoff(ctx context.Context, in protocol.ApplyWorktreeHandoffRequest, complete operation.Reply[workspace.WorktreeHandoff]) error {
	return s.taskWorktrees.ApplyHandoff(ctx, in.ID, in.Digest, complete)
}

func (s *Handler) ReleaseTaskWorktree(ctx context.Context, in protocol.TaskWorktreeRequest) error {
	var ack struct{}
	return taskWorktreeError(s.releaseTaskWorktree(ctx, in, operation.DirectReply(&ack)))
}

func (s *Handler) releaseTaskWorktree(ctx context.Context, in protocol.TaskWorktreeRequest, complete operation.Reply[struct{}]) error {
	return s.taskWorktrees.Release(ctx, in.ID, complete)
}

func presentWorktreeHandoff(preview workspace.WorktreeHandoff) *protocol.WorktreeHandoff {
	return &protocol.WorktreeHandoff{
		Worktree: presentTaskWorktree(preview.Worktree), Branch: preview.Branch, Head: preview.Head,
		Digest: preview.Digest, Patch: strings.ToValidUTF8(preview.Patch, "�"),
	}
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
