package hooks

import (
	"context"
	"fmt"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/samber/lo"
)

type roots interface {
	ResolveRoot(cwd string) (string, error)
}

type Inspector interface {
	Inspect(ctx context.Context, cwd string) (Inspection, error)
}

type TrustStore interface {
	Trust(ctx context.Context, projectRoot string) error
	Untrust(ctx context.Context, projectRoot string) error
}

type Catalog struct {
	roots         roots
	inspector     Inspector
	trust         TrustStore
	invalidations invalidation.Publish
}

type CatalogView struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []Resolved
}

type Resolved struct {
	Hook   hookdata.Hook
	Active bool
}

func NewCatalog(roots roots, inspector Inspector, trust TrustStore, invalidations invalidation.Publish) (*Catalog, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{name: "root resolver", value: roots},
		{name: "inspector", value: inspector},
		{name: "trust store", value: trust},
		{name: "invalidations", value: invalidations},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("hooks: %s is required", required.name)
		}
	}
	return &Catalog{roots: roots, inspector: inspector, trust: trust, invalidations: invalidations}, nil
}

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
			Hook: hook, Active: hook.Scope == hookdata.ScopeGlobal || inspection.ProjectTrusted,
		})
	}
	return resolved, nil
}

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
