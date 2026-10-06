package accounting

import (
	"math"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

func TestTokensAdd(t *testing.T) {
	tests := []struct {
		name string
		base Tokens
		add  Tokens
		want Tokens
	}{
		{
			name: "empty rollup",
			add:  Tokens{InputTokens: 10, OutputTokens: 4, ReasoningTokens: 2, CacheReadTokens: 3, CacheWriteTokens: 1},
			want: Tokens{InputTokens: 10, OutputTokens: 4, ReasoningTokens: 2, CacheReadTokens: 3, CacheWriteTokens: 1},
		},
		{
			name: "existing rollup",
			base: Tokens{InputTokens: 5, OutputTokens: 2, ReasoningTokens: 1},
			add:  Tokens{InputTokens: 7, OutputTokens: 3, ReasoningTokens: 2},
			want: Tokens{InputTokens: 12, OutputTokens: 5, ReasoningTokens: 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.base.Add(tt.add)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("Tokens = %+v, want %+v", got, tt.want)
			}
			total, err := got.Total()
			if err != nil || total != tt.want.InputTokens+tt.want.OutputTokens {
				t.Fatalf("Total() = %d, %v; want %d", total, err, tt.want.InputTokens+tt.want.OutputTokens)
			}
		})
	}
}

func TestTokenAndModelUsageRejectOverflow(t *testing.T) {
	if _, err := (Tokens{InputTokens: math.MaxInt64}).Add(Tokens{InputTokens: 1}); err == nil {
		t.Fatal("Tokens.Add accepted overflow")
	}
	if _, err := (Tokens{InputTokens: math.MaxInt64, OutputTokens: 1}).Total(); err == nil {
		t.Fatal("Tokens.Total accepted overflow")
	}
	left := ModelUsage{Model: "model", Calls: math.MaxInt, Cost: mustCost(t, 0)}
	right := ModelUsage{Model: "model", Calls: 1, Cost: mustCost(t, 0)}
	if _, err := left.Add(right); err == nil {
		t.Fatal("ModelUsage.Add accepted call-count overflow")
	}
}

func mustCost(t *testing.T, usd float64) Cost {
	t.Helper()
	cost, err := NewCost(usd)
	if err != nil {
		t.Fatalf("NewCost(%g): %v", usd, err)
	}
	return cost
}

func TestSnapshotTotalAggregatesModelsWithCapacityChecks(t *testing.T) {
	snapshot := Snapshot{Models: []ModelUsage{
		{
			Model: "alpha",
			Tokens: Tokens{
				InputTokens:     3,
				OutputTokens:    2,
				ReasoningTokens: 1,
			},
			Cost:  mustCost(t, 0.25),
			Calls: 1,
		},
		{
			Model: "beta",
			Tokens: Tokens{
				InputTokens:  5,
				OutputTokens: 1,
			},
			Cost:  mustCost(t, 0.5),
			Calls: 2,
		},
	}}
	total, err := snapshot.Total()
	if err != nil {
		t.Fatalf("Total: %v", err)
	}
	if total.InputTokens != 8 ||
		total.OutputTokens != 3 ||
		total.ReasoningTokens != 1 ||
		total.Calls != 3 {
		t.Fatalf("total = %+v", total)
	}
	if cost, ok := total.Cost.USD(); !ok || cost != 0.75 {
		t.Fatalf("total cost = %g, %t; want 0.75, true", cost, ok)
	}

	overflow := Snapshot{Models: []ModelUsage{
		{Model: "alpha", Tokens: Tokens{InputTokens: math.MaxInt64}, Calls: 1},
		{Model: "beta", Tokens: Tokens{InputTokens: 1}, Calls: 1},
	}}
	if _, err := overflow.Total(); err == nil {
		t.Fatal("overflowing snapshot aggregate was accepted")
	}
}

func TestUsageRejectsInvalidModelIdentities(t *testing.T) {
	t.Parallel()

	for _, model := range []string{
		"bad model",
		"model\x00shadow",
		strings.Repeat("m", modelref.MaximumModelIdentityCharacters+1),
	} {
		if err := (Usage{ByModel: map[string]Totals{model: {}}}).Validate(); err == nil {
			t.Fatalf("Usage.Validate accepted model identity %q", model)
		}
		if err := (Snapshot{Models: []ModelUsage{{Model: model, Calls: 1}}}).Validate(); err == nil {
			t.Fatalf("Snapshot.Validate accepted model identity %q", model)
		}
	}
}

func TestCostPreservesPricingAvailability(t *testing.T) {
	unpriced := Cost{}
	pricedZero := mustCost(t, 0)
	if unpriced.Equal(pricedZero) {
		t.Fatal("unpriced cost equals an explicitly priced zero")
	}
	if value, ok := pricedZero.USD(); !ok || value != 0 {
		t.Fatalf("priced zero = %g, %t; want 0, true", value, ok)
	}
	if value, ok := unpriced.USD(); ok || value != 0 {
		t.Fatalf("unpriced = %g, %t; want 0, false", value, ok)
	}

	partial, err := mustCost(t, 1.25).Add(unpriced)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, ok := partial.USD(); ok {
		t.Fatal("aggregate with an unpriced component was reported as priced")
	}
	if err := unpriced.ValidateAdvanceFrom(pricedZero); err != nil {
		t.Fatalf("priced aggregate could not become unavailable: %v", err)
	}
	if err := pricedZero.ValidateAdvanceFrom(unpriced); err == nil {
		t.Fatal("unavailable aggregate became exact")
	}
	if err := mustCost(t, 0.5).ValidateAdvanceFrom(mustCost(t, 1)); err == nil {
		t.Fatal("priced aggregate regressed")
	}
}

func TestTotalsAllowPricingToBecomeUnavailableWithoutLosingUsage(t *testing.T) {
	priced := 0.25
	previous := Totals{InputTokens: 10, CostUSD: &priced}
	next := Totals{InputTokens: 20}
	if err := next.ValidateAdvanceFrom(previous); err != nil {
		t.Fatalf("ValidateAdvanceFrom: %v", err)
	}
	if err := previous.ValidateAdvanceFrom(next); err == nil {
		t.Fatal("unknown cumulative pricing became exact")
	}
}
