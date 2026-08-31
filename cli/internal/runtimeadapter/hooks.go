package runtimeadapter

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/cli/internal/workspace"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type hookBinding interface {
	ListHooks(context.Context, protocol.ListHooksRequest, flameruntime.CallOptions) (*protocol.HooksListResult, error)
	SetHookTrust(context.Context, protocol.SetHookTrustRequest, flameruntime.CommandOptions) error
}

type hookAdapter struct{ runtime *Connection }

var _ workspace.HookService = (*hookAdapter)(nil)

func (h *hookAdapter) Catalog(ctx context.Context, workspacePath string) (workspace.HookCatalog, error) {
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
	if !hookProjectRootContainsWorkspace(result.ProjectRoot, workspacePath) {
		return workspace.HookCatalog{}, runtimeContractViolation(
			"list hooks for workspace %q returned unrelated project root %q",
			workspacePath,
			result.ProjectRoot,
		)
	}
	catalog := workspace.HookCatalog{
		ProjectRoot: result.ProjectRoot, ProjectTrusted: result.ProjectTrusted,
		Hooks: make([]workspace.LifecycleHook, 0, len(result.Hooks)),
	}
	for _, value := range result.Hooks {
		catalog.Hooks = append(catalog.Hooks, workspace.LifecycleHook{
			Event: workspace.HookEvent(value.Event), Matcher: value.Matcher,
			Command: value.Command, Inject: value.Inject, TimeoutMillis: value.TimeoutMillis,
			Scope: workspace.HookScope(value.Scope), Source: value.Source, Active: value.Active,
		})
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

func (h *hookAdapter) SetProjectTrust(ctx context.Context, projectRoot string, trusted bool) error {
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
