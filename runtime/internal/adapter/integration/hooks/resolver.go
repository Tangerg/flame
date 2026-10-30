// Package hooks admits installed rule sources under Runtime project trust.
package hooks

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/project"
	"github.com/samber/lo"
)

// Source supplies one exact installed provider. Absence contributes no hooks.
type Source interface {
	AcquireHooks(context.Context) (Use, error)
}

// Use retains the captured provider until discovery and every event have joined.
type Use interface {
	Discover(context.Context, hookprovider.Discovery) ([]hook.Hook, error)
	RunHookCommand(context.Context, apphooks.CommandRequest) apphooks.CommandResult
	Close() error
}

type Resolver struct {
	home    string
	trusted func(context.Context, string) (bool, error)
	source  Source
	onError func(context.Context, string, error)
}

func NewResolver(home string, trusted func(context.Context, string) (bool, error), source Source, onError func(context.Context, string, error)) (*Resolver, error) {
	if lo.IsNil(source) || (home != "" && !filepath.IsAbs(home)) {
		return nil, errors.New("hooks: installed source and absolute home are required")
	}
	return &Resolver{home: home, trusted: trusted, source: source, onError: onError}, nil
}

func (r *Resolver) For(ctx context.Context, cwd string) (*apphooks.Bound, error) {
	if r == nil || cwd == "" {
		return nil, nil
	}
	inspection, use, err := r.discover(ctx, cwd, false)
	if err != nil || use == nil {
		return nil, err
	}
	return apphooks.NewBound(inspection.Hooks, apphooks.NewRunner(use, r.onError), use.Close), nil
}

func (r *Resolver) Inspect(ctx context.Context, cwd string) (_ apphooks.Inspection, returned error) {
	if r == nil || cwd == "" {
		return apphooks.Inspection{}, nil
	}
	inspection, use, err := r.discover(ctx, cwd, true)
	if use != nil {
		defer func() { returned = errors.Join(returned, use.Close()) }()
	}
	return inspection, err
}

func (r *Resolver) discover(ctx context.Context, cwd string, includeUntrusted bool) (_ apphooks.Inspection, _ Use, returned error) {
	root, err := project.Root(cwd)
	if err != nil {
		return apphooks.Inspection{}, nil, fmt.Errorf("hooks: locate project root: %w", err)
	}
	inspection := apphooks.Inspection{ProjectRoot: root}
	if r.trusted != nil {
		inspection.ProjectTrusted, err = r.trusted(ctx, root)
		if err != nil {
			return apphooks.Inspection{}, nil, err
		}
	}
	use, err := r.source.AcquireHooks(ctx)
	if err != nil || lo.IsNil(use) {
		return inspection, nil, err
	}
	defer func() {
		if returned != nil {
			returned = errors.Join(returned, use.Close())
		}
	}()
	query := hookprovider.Discovery{CWD: cwd, ProjectRoot: root, Home: r.home, IncludeProject: includeUntrusted || inspection.ProjectTrusted}
	inspection.Hooks, err = use.Discover(ctx, query)
	if err != nil {
		return apphooks.Inspection{}, nil, fmt.Errorf("hooks: discover installed rules: %w", err)
	}
	if err := inspection.ValidateFor(cwd); err != nil {
		return apphooks.Inspection{}, nil, err
	}
	if !query.IncludeProject && slices.ContainsFunc(inspection.Hooks, func(value hook.Hook) bool { return value.Scope == hook.ScopeProject }) {
		return apphooks.Inspection{}, nil, errors.New("hooks: source returned untrusted project rules")
	}
	return inspection, use, nil
}
