package workspace

import (
	"context"
	"fmt"

	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
)

// HookInspector resolves lifecycle hooks and project trust for a working directory.
type HookInspector interface {
	Inspect(ctx context.Context, cwd string) (apphooks.Inspection, error)
}

// HookTrustStore durably mutates project hook trust.
type HookTrustStore interface {
	Trust(ctx context.Context, projectRoot string) error
	Untrust(ctx context.Context, projectRoot string) error
}

// Hooks owns lifecycle-hook inspection and trust decisions.
type Hooks struct {
	scope         *Scope
	inspector     HookInspector
	trust         HookTrustStore
	invalidations invalidation.Publish
}

// HookInspection is the resolved hook view after applying trust policy.
type HookInspection struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []ResolvedHook
}

type ResolvedHook struct {
	Hook   hooks.Hook
	Active bool
}

func NewHooks(scope *Scope, inspector HookInspector, trust HookTrustStore, invalidations invalidation.Publish) (*Hooks, error) {
	for _, dependency := range []struct {
		name  string
		value any
	}{
		{name: "scope", value: scope},
		{name: "inspector", value: inspector},
		{name: "trust store", value: trust},
	} {
		if missingDependency(dependency.value) {
			return nil, fmt.Errorf("workspace: hooks %s is required", dependency.name)
		}
	}
	return &Hooks{scope: scope, inspector: inspector, trust: trust, invalidations: invalidations}, nil
}

// Inspect returns lifecycle hooks and their effective activation state.
func (h *Hooks) Inspect(ctx context.Context, cwd string) (HookInspection, error) {
	root, err := h.scope.root(cwd)
	if err != nil {
		return HookInspection{}, err
	}
	inspection, err := h.inspector.Inspect(ctx, root)
	if err != nil {
		return HookInspection{}, err
	}
	if err := inspection.ValidateFor(root); err != nil {
		return HookInspection{}, err
	}
	resolved := HookInspection{
		ProjectRoot: inspection.ProjectRoot, ProjectTrusted: inspection.ProjectTrusted,
		Hooks: make([]ResolvedHook, 0, len(inspection.Hooks)),
	}
	for _, hook := range inspection.Hooks {
		resolved.Hooks = append(resolved.Hooks, ResolvedHook{
			Hook: hook, Active: hook.Scope == hooks.ScopeGlobal || inspection.ProjectTrusted,
		})
	}
	return resolved, nil
}

// SetProjectTrust changes whether project hooks may run.
func (h *Hooks) SetProjectTrust(ctx context.Context, projectRoot string, trusted bool) error {
	root, err := h.scope.root(projectRoot)
	if err != nil {
		return err
	}
	var changeErr error
	if trusted {
		changeErr = h.trust.Trust(ctx, root)
	} else {
		changeErr = h.trust.Untrust(ctx, root)
	}
	if changeErr == nil {
		h.invalidations.Notify(invalidation.Notice{Resource: invalidation.Hooks})
	}
	return changeErr
}
