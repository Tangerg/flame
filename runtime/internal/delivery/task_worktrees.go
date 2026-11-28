package delivery

import (
	"bytes"
	"context"
	"errors"
	"reflect"
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
	registry.externalCommand(MethodMeta{
		Name: TaskWorktreesCreate, GoName: "CreateTaskWorktree", CapabilityRules: git,
		Errors: []string{unavailable, protocol.ErrVcsUnavailable.Error()},
	}, (*Handler).createTaskWorktree, func(task workspace.TaskWorktree) *protocol.TaskWorktree {
		return new(presentTaskWorktree(task))
	}, taskWorktreeError)
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
	registerWorktreeWrite(registry, MethodMeta{
		Name: TaskWorktreesApply, GoName: "ApplyWorktreeHandoff", CapabilityRules: git, Errors: writeErrors,
	}, (*Handler).applyWorktreeHandoff, presentWorktreeHandoff)
	registerWorktreeWrite(registry, MethodMeta{
		Name: TaskWorktreesRelease, GoName: "ReleaseTaskWorktree", CapabilityRules: git, Errors: writeErrors,
	}, (*Handler).releaseTaskWorktree, func(ack struct{}) struct{} { return ack })
}

func registerWorktreeWrite[Params, Value, Result any](
	registry *Registry,
	meta MethodMeta,
	prepare func(*Handler, context.Context, Params, operation.ExternalReply[Value]) error,
	present func(Value) Result,
) {
	meta.Result = reflect.TypeFor[Result]()
	if meta.Result == acknowledgementType {
		meta.Result = nil
	}
	registry.requiredReplayCommand(meta, func(owner *Handler, ctx context.Context, in Params, call keyedCall) protocol.OperationResult {
		payload, err := call.replay.InvokeOwnedExternal(ctx, call.key, call.fingerprint, call.admit, owner.taskWorktrees.Receipts(),
			func(complete operation.ExternalCompletion) error {
				return prepare(owner, ctx, in, func(ctx context.Context, value Value, write operation.ExternalExecution) (bool, error) {
					success, err := encodeAtomicResult(call.method, present(value))
					if err != nil {
						return false, err
					}
					receipt, err := complete(ctx, write, success)
					return bytes.Equal(receipt, success), err
				})
			}, EncodeTaskWriteRefusal)
		return call.result(ctx, payload, err)
	})
}

func EncodeTaskWriteRefusal(cause error) ([]byte, error) {
	return encodeAcknowledgement(taskWorktreeError(cause))
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

func (s *Handler) applyWorktreeHandoff(ctx context.Context, in protocol.ApplyWorktreeHandoffRequest, complete operation.ExternalReply[workspace.WorktreeHandoff]) error {
	return s.taskWorktrees.ApplyHandoff(ctx, in.ID, in.Digest, complete)
}

func (s *Handler) releaseTaskWorktree(ctx context.Context, in protocol.TaskWorktreeRequest, complete operation.ExternalReply[struct{}]) error {
	return s.taskWorktrees.Release(ctx, in.ID, complete)
}

func presentWorktreeHandoff(preview workspace.WorktreeHandoff) *protocol.WorktreeHandoff {
	return &protocol.WorktreeHandoff{
		Worktree: presentTaskWorktree(preview.Worktree), Branch: preview.Branch, Head: preview.Head,
		Digest: preview.Digest, Patch: preview.Patch,
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
	case errors.Is(err, workspace.ErrTaskWorktreeInput), errors.Is(err, workspace.ErrWorktreePatchEncoding):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	default:
		return wireWorkspaceError(err)
	}
}
