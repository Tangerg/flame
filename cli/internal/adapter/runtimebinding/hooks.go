package runtimebinding

import (
	"context"
	"errors"
	"path/filepath"
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
	if !filepath.IsAbs(workspacePath) {
		return workspace.HookCatalog{}, errors.New("list hooks: workspace is not absolute")
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
	if err := protocol.ValidateWireTree(*result); err != nil {
		return workspace.HookCatalog{}, runtimeContractViolation("list hooks returned an invalid wire result: %v", err)
	}
	if !hookProjectRootContainsWorkspace(result.ProjectRoot, workspacePath) {
		return workspace.HookCatalog{}, runtimeContractViolation(
			"list hooks for workspace %q returned unrelated project root %q",
			workspacePath,
			result.ProjectRoot,
		)
	}
	catalog := workspace.HookCatalog{
		ProjectRoot: result.ProjectRoot, ProjectTrusted: result.ProjectTrusted,
		Hooks: slices.Clone(result.Hooks),
	}
	if err := catalog.Validate(); err != nil {
		return workspace.HookCatalog{}, runtimeContractViolation("list hooks returned an invalid catalog: %v", err)
	}
	return catalog, nil
}

func hookProjectRootContainsWorkspace(projectRoot, workspace string) bool {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" || !filepath.IsAbs(projectRoot) {
		return false
	}
	relative, err := filepath.Rel(filepath.Clean(projectRoot), filepath.Clean(workspace))
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (h *Hooks) SetProjectTrust(ctx context.Context, projectRoot string, trusted bool) error {
	r := h.runtime
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return errors.New("set hook trust: project root is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.hooks.SetHookTrust(ctx, protocol.SetHookTrustRequest{
		ProjectRoot: projectRoot, Trusted: trusted,
	}, options))
}
