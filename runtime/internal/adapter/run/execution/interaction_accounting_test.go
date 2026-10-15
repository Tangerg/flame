package execution

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	"github.com/Tangerg/scope/core/chat"
)

func TestWaitingAccountingRestoresUnreportedCallsWithoutInventingUsage(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprint(mixed), func(t *testing.T) {
			metrics, err := run.NewMetrics(nil, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			metrics, err = metrics.AccrueModelCall("unreported", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			counts := map[string]int{"unreported": 1}
			if mixed {
				metrics, err = metrics.AccrueModelCall("reported", &chat.Usage{InputTokens: 11, OutputTokens: 3}, new(1.25))
				if err != nil {
					t.Fatal(err)
				}
				counts["reported"] = 1
			}
			before, present := metrics.Usage()
			restored, err := accountingFromRunMetrics(metrics, counts)
			if err != nil || restored["unreported"].Calls != 1 || restored["unreported"].InputTokens != 0 {
				t.Fatalf("unreported call cannot resume: %+v, %v", restored, err)
			}
			if _, known := restored["unreported"].Cost.USD(); known {
				t.Fatal("missing usage acquired a known price")
			}
			after, afterPresent := metrics.Usage()
			if present != mixed || present != afterPresent || !reflect.DeepEqual(before, after) {
				t.Fatal("restoration originated a reported usage fact")
			}
			if mixed && (restored["reported"].InputTokens != 11 || restored["reported"].OutputTokens != 3) {
				t.Fatal("known source usage was changed")
			}
			counts["unreported"] = 2
			if _, err := accountingFromRunMetrics(metrics, counts); err == nil {
				t.Fatal("mismatched checkpoint call counts were accepted")
			}
		})
	}
}

func interactionTestCost(usd float64) accounting.Cost {
	cost, err := accounting.NewCost(usd)
	if err != nil {
		panic(err)
	}
	return cost
}

func fixedInteractionPricing(usd float64) accounting.Pricing {
	cost := interactionTestCost(usd)
	return func(_, _ string, _ *chat.Usage) accounting.Cost { return cost }
}

func TestAccountModelCallRejectsOutOfSequenceWithoutMutatingUsage(t *testing.T) {
	var invocation interaction.ModelInvocation
	model := chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
		var present bool
		invocation, present = interaction.ModelInvocationFromContext(ctx)
		if !present {
			return nil, errors.New("model call has no invocation attribution")
		}
		return interactionUsageTextResponse("captured", 2, 1), nil
	})
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{})
	runInteractionHarness(context.Background(), t, executor, interactionTestStart(), nil)

	ledger := newInteractionAccounting(testDefaultSelection())
	processID := invocation.Relation().ProcessID()
	ledger.usageByProcess[processID] = map[string]accounting.ModelUsage{
		"test-model": {Model: "test-model", Calls: 1},
	}
	if err := ledger.prepareModelContext(invocation, 100); err != nil {
		t.Fatal(err)
	}
	before := maps.Clone(ledger.usageByProcess[processID])

	_, err := ledger.accountModelCall(invocation, "duplicate", interactionUsageTextResponse("duplicate", 3, 1), accounting.Cost{})
	if err == nil || !strings.Contains(err.Error(), "differs from accounted calls") {
		t.Fatalf("account duplicate call error = %v", err)
	}
	if after := ledger.usageByProcess[processID]; !reflect.DeepEqual(after, before) {
		t.Fatalf("rejected model call mutated usage: before=%+v after=%+v", before, after)
	}
	ledger.mu.Lock()
	_, prepared := ledger.preparedContextByProcess[processID]
	ledger.mu.Unlock()
	if !prepared {
		t.Fatal("rejected model call consumed its prepared context")
	}
	ledger.discardPreparedModelContext(invocation)
	if err := ledger.prepareModelContext(invocation, 100); err != nil {
		t.Fatalf("prepare after discarded model context: %v", err)
	}
}

func TestAdvanceProcessUsageRejectsOverflowWithoutMutatingInput(t *testing.T) {
	current := map[string]accounting.ModelUsage{
		"test-model": {
			Model: "test-model", Tokens: accounting.Tokens{InputTokens: math.MaxInt64},
			Cost: interactionTestCost(0), Calls: 1,
		},
	}
	before := current["test-model"]
	_, _, _, err := advanceProcessUsage(current, accounting.ModelUsage{
		Model: "test-model", Tokens: accounting.Tokens{InputTokens: 1},
		Cost: interactionTestCost(0), Calls: 1,
	}, 2)
	if err == nil {
		t.Fatal("overflowing process usage was accepted")
	}
	if current["test-model"] != before {
		t.Fatalf("failed aggregation mutated input: before=%+v after=%+v", before, current["test-model"])
	}
}

func TestModelCallUsageDistinguishesUnavailableFromReportedZero(t *testing.T) {
	for _, reported := range []bool{false, true} {
		t.Run(fmt.Sprint(reported), func(t *testing.T) {
			response := interactionUsageTextResponse("done", 0, 0)
			if !reported {
				response.Metadata = nil
			}
			model := &observationScriptModel{responses: []*chat.Response{response}}
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{})
			calls := payloadsOf[runs.ModelCallCompleted](runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil))
			if len(calls) != 1 {
				t.Fatalf("model calls = %d", len(calls))
			}
			if (calls[0].ReportedUsage != nil) != reported {
				t.Fatalf("reported usage = %+v, reported=%t", calls[0].ReportedUsage, reported)
			}
		})
	}
}
