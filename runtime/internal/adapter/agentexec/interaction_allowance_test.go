package agentexec

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestInteractionAllowanceStopsBeforeTheNextModelCall(t *testing.T) {
	tests := []struct {
		name    string
		limits  run.LimitValues
		pricing accounting.Pricing
	}{
		{
			name: "tokens",
			limits: run.LimitValues{
				MaxTotalTokens: testsupport.Pointer[int64](1),
			},
		},
		{
			name: "cost",
			limits: run.LimitValues{
				MaxBudgetUSD: testsupport.Pointer(0.2),
			},
			pricing: fixedInteractionPricing(0.25),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool, err := toolcontract.NewFunc(toolcontract.FuncConfig{
				Name: "echo", Description: "Return the supplied value.",
			}, func(_ context.Context, input struct {
				Value string `json:"value"`
			}) (string, error) {
				return input.Value, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			model := &observationScriptModel{responses: []*chat.Response{
				interactionToolResponse(chat.ToolCall{
					ID: "provider_call", Name: "echo", Arguments: `{"value":"done"}`,
				}, 7, 2),
				interactionUsageTextResponse("must not be called", 1, 1),
			}}
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
				ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{tool}}},
				ToolInterpreter: testInteractionToolInterpreter{}, ToolPresenter: testInteractionToolPresenter{},
				ToolAuthorizer: allowInteractionTools{}, Pricing: test.pricing,
			})
			start := interactionTestStart()
			start.Limits = testsupport.MustRunLimits(test.limits)
			events := runInteractionHarness(t.Context(), t, executor, start, nil)

			ended := payloadsOf[runs.SegmentEnded](events)
			if len(ended) != 1 || ended[0].Reason != run.OutcomeMaxBudget || ended[0].Failure != nil ||
				ended[0].Usage == nil || ended[0].Usage.Steps != 1 {
				t.Fatalf("SegmentEnded = %#v, want one metered max-budget terminal", ended)
			}
			if len(payloadsOf[runs.ModelCallCompleted](events)) != 1 ||
				len(payloadsOf[runs.ToolCallFinished](events)) != 1 {
				t.Fatalf("events admitted work after the allowance: %#v", events)
			}
		})
	}
}

func TestInteractionAllowanceLetsBoundaryFinalAnswerComplete(t *testing.T) {
	model := &observationScriptModel{responses: []*chat.Response{
		interactionUsageTextResponse("done", 7, 2),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{})
	start := interactionTestStart()
	start.Limits = testsupport.MustRunLimits(run.LimitValues{
		MaxTotalTokens: testsupport.Pointer[int64](1),
	})
	events := runInteractionHarness(t.Context(), t, executor, start, nil)
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted ||
		len(payloadsOf[runs.ModelCallCompleted](events)) != 1 {
		t.Fatalf("boundary final answer = %#v, want completed", ended)
	}
}

func TestInteractionAllowanceRejectsUnpricedCostLimitBeforeCallingProvider(t *testing.T) {
	var calls int
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		calls++
		return interactionUsageTextResponse("unexpected", 1, 1), nil
	})
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		Pricing: func(_, _ string, _ *chat.Usage) accounting.Cost { return accounting.Cost{} },
	})
	start := interactionTestStart()
	start.Limits = testsupport.MustRunLimits(run.LimitValues{
		MaxBudgetUSD: testsupport.Pointer(1.0),
	})
	if _, err := executor.StageRoot(t.Context(), start); !errors.Is(err, runs.ErrInvalidRunLimit) {
		t.Fatalf("StageRoot error = %v, want ErrInvalidRunLimit", err)
	}
	if calls != 0 {
		t.Fatalf("provider calls = %d, want zero", calls)
	}
}

func TestInteractionAllowanceFailsClosedWhenServedModelPricingDisappears(t *testing.T) {
	limits := testsupport.MustRunLimits(run.LimitValues{
		MaxBudgetUSD: testsupport.Pointer(1.0),
	})
	allowance, err := newInteractionAllowance(
		limits,
		testDefaultSelection(),
		fixedInteractionPricing(0),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := allowance.admit(accounting.Snapshot{Models: []accounting.ModelUsage{{
		Model: "served-alias", Calls: 1,
	}}}); !errors.Is(err, errInteractionAllowanceDenied) {
		t.Fatalf("admit error = %v, want allowance denial", err)
	}
	if allowance.terminal() != interactionAllowancePricingUnavailable {
		t.Fatalf("terminal = %d, want pricing unavailable", allowance.terminal())
	}
}

func TestInteractionAllowanceOwnsTreeWideSteps(t *testing.T) {
	limits := testsupport.MustRunLimits(run.LimitValues{
		MaxSteps: testsupport.Pointer(1),
	})
	allowance, err := newInteractionAllowance(limits, testDefaultSelection(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := allowance.admit(accounting.Snapshot{Models: []accounting.ModelUsage{{
		Model: "child-served-model", Calls: 1,
	}}}); !errors.Is(err, errInteractionAllowanceDenied) {
		t.Fatalf("admit error = %v, want allowance denial", err)
	}
	if allowance.terminal() != interactionAllowanceStepsExhausted {
		t.Fatalf("terminal = %d, want steps exhausted", allowance.terminal())
	}
	if err := allowance.admit(accounting.Snapshot{}); !errors.Is(err, errInteractionAllowanceDenied) {
		t.Fatalf("admit after terminal stop = %v, want sticky allowance denial", err)
	}
	if allowance.terminal() != interactionAllowanceStepsExhausted {
		t.Fatalf("terminal reopened as %d", allowance.terminal())
	}
}
