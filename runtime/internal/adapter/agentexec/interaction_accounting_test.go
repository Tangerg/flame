package agentexec

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/agent/interaction"
	"github.com/Tangerg/scope/core/chat"
)

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

	ledger := newInteractionAccounting(testDefaultSelection(), nil)
	processID := invocation.Relation().ProcessID()
	ledger.usageByProcess[processID] = map[string]accounting.ModelUsage{
		"test-model": {Model: "test-model", Calls: 1},
	}
	if err := ledger.prepareModelContext(invocation, 100); err != nil {
		t.Fatal(err)
	}
	before, err := ledger.snapshot()
	if err != nil {
		t.Fatal(err)
	}

	_, err = ledger.accountModelCall(invocation, "duplicate", interactionUsageTextResponse("duplicate", 3, 1))
	if err == nil || !strings.Contains(err.Error(), "differs from accounted calls") {
		t.Fatalf("account duplicate call error = %v", err)
	}
	after, snapshotErr := ledger.snapshot()
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("rejected model call mutated usage: before=%+v after=%+v", before, after)
	}
	ledger.mu.Lock()
	_, prepared := ledger.preparedContextByProcess[processID]
	ledger.mu.Unlock()
	if !prepared {
		t.Fatal("rejected model call consumed its prepared context")
	}
}
