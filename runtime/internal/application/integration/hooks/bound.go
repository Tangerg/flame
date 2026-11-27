package hooks

import (
	"context"
	"errors"
	"fmt"
	"slices"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
)

// Evaluate returns advice admitted by the provider boundary.
type Evaluate func(context.Context, hookprovider.Evaluation) (hookprovider.Advice, error)
type Bound struct {
	rules    []hookdata.Hook
	evaluate Evaluate
	report   func(context.Context, string, error)
	close    func() error
}

func NewBound(rules []hookdata.Hook, evaluate Evaluate, report func(context.Context, string, error), release func() error) (*Bound, error) {
	if evaluate == nil || report == nil || release == nil {
		return nil, errors.New("hooks: bound rules require an evaluator, a diagnostic sink and a release")
	}
	return &Bound{rules: slices.Clone(rules), evaluate: evaluate, report: report, close: release}, nil
}

// A workspace without lifecycle rules still gets a binding, so callers
// never branch on its absence.
func Unbound() *Bound {
	return &Bound{close: func() error { return nil }}
}

func (b *Bound) Run(ctx context.Context, in hookdata.Input) (hookdata.CommandDecision, error) {
	if len(b.rules) == 0 {
		return hookdata.CommandDecision{Verdict: hookdata.CommandAllow}, nil
	}
	projected, err := in.CommandProjection()
	request := hookprovider.Evaluation{Rules: b.rules, Input: projected}
	if err != nil {
		request.DeclarativeOnly = true
		request.Input = hookdata.Input{Event: in.Event, CWD: in.CWD}
		if in.Tool != nil {
			request.Input.Tool = &hookdata.ToolInput{Name: in.Tool.Name}
		}
		b.report(ctx, "lifecycle event", err)
	}
	advice, err := b.evaluate(ctx, request)
	if err != nil {
		return hookdata.CommandDecision{}, fmt.Errorf("hooks: evaluate %s: %w", in.Event, err)
	}
	for _, issue := range advice.Diagnostics {
		b.report(ctx, issue.Source, errors.New(issue.Message))
	}
	return advice.Decision, nil
}

type Inspection struct {
	RepositoryRoot    string
	RepositoryTrusted bool
	Hooks             []hookdata.Hook
}

func (b *Bound) Close() error { return b.close() }
