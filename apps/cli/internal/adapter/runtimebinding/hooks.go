package runtimebinding

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type hookBinding interface {
	ListHooks(context.Context, protocol.ListHooksRequest, protocol.CallOptions) (*protocol.HooksListResult, error)
	SetHookTrust(context.Context, protocol.SetHookTrustRequest, protocol.CommandOptions) error
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
		RepositoryRoot: result.RepositoryRoot, RepositoryTrusted: result.RepositoryTrusted,
		Hooks: slices.Clone(result.Hooks),
	}, nil
}

func (h *Hooks) SetRepositoryTrust(ctx context.Context, repositoryRoot string, trusted bool) error {
	r := h.runtime
	repositoryRoot = strings.TrimSpace(repositoryRoot)
	if repositoryRoot == "" {
		return errors.New("set hook trust: repository root is empty")
	}
	options := r.commandOptions()
	return classifyError(r.hooks.SetHookTrust(ctx, protocol.SetHookTrustRequest{
		RepositoryRoot: repositoryRoot, Trusted: trusted,
	}, options))
}
