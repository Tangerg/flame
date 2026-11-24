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
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/repository"
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
	inspection, use, err := r.discover(ctx, cwd, trustedRepositoryRules)
	if err != nil {
		return nil, err
	}
	if use == nil {
		return apphooks.Unbound(), nil
	}
	evaluate := use.Evaluate
	if slices.ContainsFunc(inspection.Hooks, func(rule hook.Hook) bool { return rule.Scope == hook.ScopeRepository }) {
		evaluate = func(ctx context.Context, event hookprovider.Evaluation) (hookprovider.Advice, error) {
			trusted, err := r.trusted(ctx, inspection.RepositoryRoot)
			if err != nil {
				return hookprovider.Advice{}, err
			}
			if !trusted {
				event.Rules = slices.DeleteFunc(slices.Clone(event.Rules), func(rule hook.Hook) bool { return rule.Scope == hook.ScopeRepository })
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
	inspection, use, err := r.discover(ctx, cwd, everyRepositoryRule)
	if use != nil {
		defer func() { returned = errors.Join(returned, use.Close()) }()
	}
	return inspection, err
}

// Inspection lists an untrusted repository's rules so the user can decide on trust;
// binding never loads them.
type repositoryRuleSelection int

const (
	trustedRepositoryRules repositoryRuleSelection = iota
	everyRepositoryRule
)

func (r *Resolver) discover(ctx context.Context, cwd string, selection repositoryRuleSelection) (_ apphooks.Inspection, _ Use, returned error) {
	root, err := repository.Root(cwd)
	if err != nil {
		return apphooks.Inspection{}, nil, fmt.Errorf("hooks: locate repository root: %w", err)
	}
	inspection := apphooks.Inspection{RepositoryRoot: root}
	inspection.RepositoryTrusted, err = r.trusted(ctx, root)
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
	query := hookprovider.Discovery{CWD: cwd, RepositoryRoot: root, Home: r.home, IncludeRepository: selection == everyRepositoryRule || inspection.RepositoryTrusted}
	inspection.Hooks, err = use.Discover(ctx, query)
	if err != nil {
		return apphooks.Inspection{}, nil, fmt.Errorf("hooks: discover installed rules: %w", err)
	}
	if err := inspection.ValidateFor(cwd); err != nil {
		return apphooks.Inspection{}, nil, err
	}
	if !query.IncludeRepository && slices.ContainsFunc(inspection.Hooks, func(value hook.Hook) bool { return value.Scope == hook.ScopeRepository }) {
		return apphooks.Inspection{}, nil, errors.New("hooks: source returned untrusted repository rules")
	}
	return inspection, use, nil
}
