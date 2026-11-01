package hooks

import (
	"context"
	"errors"
	"slices"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	domain "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
)

type Evaluate func(context.Context, hookprovider.Evaluation) (hookprovider.Advice, error)
type Bound struct {
	rules    []hookdata.Hook
	evaluate Evaluate
	report   func(context.Context, string, error)
	close    func() error
}

func NewBound(rules []hookdata.Hook, evaluate Evaluate, report func(context.Context, string, error), close func() error) *Bound {
	return &Bound{rules: slices.Clone(rules), evaluate: evaluate, report: report, close: close}
}

func (b *Bound) Run(ctx context.Context, in hookdata.Input) domain.Decision {
	if b == nil || b.evaluate == nil || len(b.rules) == 0 {
		return domain.Decision{}
	}
	projected, err := in.CommandProjection()
	request := hookprovider.Evaluation{Rules: b.rules, Input: projected}
	if err != nil {
		request.DeclarativeOnly = true
		request.Input = hookdata.Input{Event: in.Event, CWD: in.CWD}
		if in.Tool != nil {
			request.Input.Tool = &hookdata.ToolInput{Name: in.Tool.Name}
		}
		b.diagnostic(ctx, "lifecycle event", err)
	}
	advice, err := b.evaluate(ctx, request)
	if err == nil {
		err = advice.Validate()
	}
	if err != nil {
		b.diagnostic(ctx, "lifecycle provider", err)
		return domain.Decision{Verdict: domain.VerdictBlock, Reason: err.Error()}
	}
	for _, issue := range advice.Diagnostics {
		b.diagnostic(ctx, issue.Source, errors.New(issue.Message))
	}
	out := advice.Decision
	verdict := domain.VerdictAllow
	switch out.Verdict {
	case hookdata.CommandDeny:
		verdict = domain.VerdictBlock
	case hookdata.CommandAsk:
		verdict = domain.VerdictAsk
	}
	return domain.Decision{Verdict: verdict, Reason: out.Reason, InjectContext: out.InjectContext, RewriteArguments: out.RewriteArguments}
}
func (b *Bound) diagnostic(ctx context.Context, source string, err error) {
	if b.report != nil {
		b.report(ctx, source, err)
	}
}

// Inspection is the read-only view of a cwd's hooks for a management surface
// (hooks.list): every discovered hook (trusted or not), the project
// root that gates the project-scope ones, and whether it's currently trusted.
type Inspection struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []hookdata.Hook
}

func (b *Bound) Close() error {
	if b != nil && b.close != nil {
		return b.close()
	}
	return nil
}
