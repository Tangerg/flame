package runtimebinding

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type hookBinding interface {
	ListHooks(context.Context, protocol.ListHooksRequest, flameruntime.CallOptions) (*protocol.HooksListResult, error)
	SetHookTrust(context.Context, protocol.SetHookTrustRequest, flameruntime.CommandOptions) error
}

type Hooks struct{ runtime *Connection }

func (h *Hooks) Catalog(ctx context.Context, workspacePath string) (workspace.HookCatalog, error) {
	r := h.runtime
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return workspace.HookCatalog{}, errors.New("list hooks: workspace is empty")
	}
	result, err := r.hooks.ListHooks(ctx, protocol.ListHooksRequest{
		Workspace: protocol.WorkspaceRef{Path: workspacePath},
	}, r.callOptions())
	if err != nil {
		return workspace.HookCatalog{}, classifyError(err)
	}
	if result == nil {
		return workspace.HookCatalog{}, runtimeContractViolation("list hooks returned nil")
	}
	return workspace.HookCatalog{
		ProjectRoot: result.ProjectRoot, ProjectTrusted: result.ProjectTrusted,
		Hooks: slices.Clone(result.Hooks),
	}, nil
}

func (h *Hooks) SetProjectTrust(ctx context.Context, projectRoot string, trusted bool) error {
	r := h.runtime
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return errors.New("set hook trust: project root is empty")
	}
	options := r.commandOptions()
	return classifyError(r.hooks.SetHookTrust(ctx, protocol.SetHookTrustRequest{
		ProjectRoot: projectRoot, Trusted: trusted,
	}, options))
}
