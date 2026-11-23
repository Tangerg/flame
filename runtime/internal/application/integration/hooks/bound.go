package hooks

import (
	"context"
	"errors"
	"slices"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	domainhooks "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
)

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
	return &Bound{
		evaluate: func(context.Context, hookprovider.Evaluation) (hookprovider.Advice, error) {
			return hookprovider.Advice{}, nil
		},
		report: func(context.Context, string, error) {},
		close:  func() error { return nil },
	}
}

func (b *Bound) Run(ctx context.Context, in hookdata.Input) domainhooks.Decision {
	if len(b.rules) == 0 {
		return domainhooks.Decision{}
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
	if err == nil {
		err = advice.Validate()
	}
	if err != nil {
		b.report(ctx, "lifecycle provider", err)
		return domainhooks.Decision{Verdict: domainhooks.VerdictBlock, Reason: err.Error()}
	}
	for _, issue := range advice.Diagnostics {
		b.report(ctx, issue.Source, errors.New(issue.Message))
	}
	out := advice.Decision
	verdict := domainhooks.VerdictAllow
	switch out.Verdict {
	case hookdata.CommandDeny:
		verdict = domainhooks.VerdictBlock
	case hookdata.CommandAsk:
		verdict = domainhooks.VerdictAsk
	case hookdata.CommandAllow:
	}
	return domainhooks.Decision{Verdict: verdict, Reason: out.Reason, InjectContext: out.InjectContext, RewriteArguments: out.RewriteArguments}
}

type Inspection struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []hookdata.Hook
}

func (b *Bound) Close() error { return b.close() }
