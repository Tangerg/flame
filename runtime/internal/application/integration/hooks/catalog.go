package hooks

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	domain "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
)

// Roots resolves the existing workspace identity before inspection or trust changes.
type Roots interface {
	ResolveRoot(cwd string) (string, error)
}

// Inspector resolves lifecycle hooks and project trust for a working directory.
type Inspector interface {
	Inspect(ctx context.Context, cwd string) (Inspection, error)
}

// TrustStore durably mutates project hook trust.
type TrustStore interface {
	Trust(ctx context.Context, projectRoot string) error
	Untrust(ctx context.Context, projectRoot string) error
}

// Catalog owns lifecycle-hook inspection and trust decisions.
type Catalog struct {
	roots         Roots
	inspector     Inspector
	trust         TrustStore
	invalidations invalidation.Publish
}

// CatalogView is the resolved hook view after applying trust policy.
type CatalogView struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []Resolved
}

type Resolved struct {
	Hook   domain.Hook
	Active bool
}

func NewCatalog(roots Roots, inspector Inspector, trust TrustStore, invalidations invalidation.Publish) (*Catalog, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{name: "root resolver", value: roots},
		{name: "inspector", value: inspector},
		{name: "trust store", value: trust},
	} {
		if dependency.Missing(required.value) {
			return nil, fmt.Errorf("hooks: %s is required", required.name)
		}
	}
	return &Catalog{roots: roots, inspector: inspector, trust: trust, invalidations: invalidations}, nil
}

// Inspect returns lifecycle hooks and their effective activation state.
func (h *Catalog) Inspect(ctx context.Context, cwd string) (CatalogView, error) {
	root, err := h.roots.ResolveRoot(cwd)
	if err != nil {
		return CatalogView{}, err
	}
	inspection, err := h.inspector.Inspect(ctx, root)
	if err != nil {
		return CatalogView{}, err
	}
	if err := inspection.ValidateFor(root); err != nil {
		return CatalogView{}, err
	}
	resolved := CatalogView{
		ProjectRoot: inspection.ProjectRoot, ProjectTrusted: inspection.ProjectTrusted,
		Hooks: make([]Resolved, 0, len(inspection.Hooks)),
	}
	for _, hook := range inspection.Hooks {
		resolved.Hooks = append(resolved.Hooks, Resolved{
			Hook: hook, Active: hook.Scope == domain.ScopeGlobal || inspection.ProjectTrusted,
		})
	}
	return resolved, nil
}

// SetProjectTrust changes whether project hooks may run.
func (h *Catalog) SetProjectTrust(ctx context.Context, projectRoot string, trusted bool) error {
	root, err := h.roots.ResolveRoot(projectRoot)
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
