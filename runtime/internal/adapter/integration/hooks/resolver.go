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

type Source interface {
	AcquireHooks(context.Context) (Use, error)
}

type Use interface {
	Discover(context.Context, hookprovider.Discovery) ([]hook.Hook, error)
	Evaluate(context.Context, hookprovider.Evaluation) (hookprovider.Advice, error)
	Close() error
}

type Resolver struct {
	home    string
	trusted func(context.Context, string) (bool, error)
	source  Source
	onError func(context.Context, string, error)
}

func NewResolver(home string, trusted func(context.Context, string) (bool, error), source Source, onError func(context.Context, string, error)) (*Resolver, error) {
	if lo.IsNil(source) || trusted == nil || onError == nil || (home != "" && !filepath.IsAbs(home)) {
		return nil, errors.New("hooks: installed source, trust, diagnostics and absolute home are required")
	}
	return &Resolver{home: home, trusted: trusted, source: source, onError: onError}, nil
}

func (r *Resolver) For(ctx context.Context, cwd string) (*apphooks.Bound, error) {
	if cwd == "" {
		return apphooks.Unbound(), nil
	}
	inspection, use, err := r.discover(ctx, cwd, trustedProjectRules)
	if err != nil {
		return nil, err
	}
	if use == nil {
		return apphooks.Unbound(), nil
	}
	evaluate := use.Evaluate
	if slices.ContainsFunc(inspection.Hooks, func(rule hook.Hook) bool { return rule.Scope == hook.ScopeProject }) {
		evaluate = func(ctx context.Context, event hookprovider.Evaluation) (hookprovider.Advice, error) {
			trusted, err := r.trusted(ctx, inspection.ProjectRoot)
			if err != nil {
				return hookprovider.Advice{}, err
			}
			if !trusted {
				event.Rules = slices.DeleteFunc(slices.Clone(event.Rules), func(rule hook.Hook) bool { return rule.Scope == hook.ScopeProject })
			}
			if len(event.Rules) == 0 {
				return hookprovider.Advice{Decision: hook.CommandDecision{Verdict: hook.CommandAllow}}, nil
			}
			return use.Evaluate(ctx, event)
		}
	}
	bound, err := apphooks.NewBound(inspection.Hooks, evaluate, r.onError, use.Close)
	if err != nil {
		return nil, errors.Join(err, use.Close())
	}
	return bound, nil
}

func (r *Resolver) Inspect(ctx context.Context, cwd string) (_ apphooks.Inspection, returned error) {
	if cwd == "" {
		return apphooks.Inspection{}, nil
	}
	inspection, use, err := r.discover(ctx, cwd, everyProjectRule)
	if use != nil {
		defer func() { returned = errors.Join(returned, use.Close()) }()
	}
	return inspection, err
}

// Inspection lists an untrusted project's rules so the user can decide on trust;
// binding never loads them.
type projectRuleSelection int

const (
	trustedProjectRules projectRuleSelection = iota
	everyProjectRule
)

func (r *Resolver) discover(ctx context.Context, cwd string, selection projectRuleSelection) (_ apphooks.Inspection, _ Use, returned error) {
	root, err := project.Root(cwd)
	if err != nil {
		return apphooks.Inspection{}, nil, fmt.Errorf("hooks: locate project root: %w", err)
	}
	inspection := apphooks.Inspection{ProjectRoot: root}
	inspection.ProjectTrusted, err = r.trusted(ctx, root)
	if err != nil {
		return apphooks.Inspection{}, nil, err
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
	query := hookprovider.Discovery{CWD: cwd, ProjectRoot: root, Home: r.home, IncludeProject: selection == everyProjectRule || inspection.ProjectTrusted}
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
