package accounting

import (
	"math"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

func TestTokenUsageAdd(t *testing.T) {
	tests := []struct {
		name string
		base TokenUsage
		add  TokenUsage
		want TokenUsage
	}{
		{
			name: "empty rollup",
			add:  TokenUsage{PromptTokens: 10, CompletionTokens: 4, ReasoningTokens: 2, CacheReadTokens: 3, CacheWriteTokens: 1},
			want: TokenUsage{PromptTokens: 10, CompletionTokens: 4, ReasoningTokens: 2, CacheReadTokens: 3, CacheWriteTokens: 1},
		},
		{
			name: "existing rollup",
			base: TokenUsage{PromptTokens: 5, CompletionTokens: 2, ReasoningTokens: 1},
			add:  TokenUsage{PromptTokens: 7, CompletionTokens: 3, ReasoningTokens: 2},
			want: TokenUsage{PromptTokens: 12, CompletionTokens: 5, ReasoningTokens: 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.base.Add(tt.add)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("TokenUsage = %+v, want %+v", got, tt.want)
			}
			total, err := got.Total()
			if err != nil || total != tt.want.PromptTokens+tt.want.CompletionTokens {
				t.Fatalf("Total() = %d, %v; want %d", total, err, tt.want.PromptTokens+tt.want.CompletionTokens)
			}
		})
	}
}

func TestTokenAndModelUsageRejectOverflow(t *testing.T) {
	if _, err := (TokenUsage{PromptTokens: math.MaxInt64}).Add(TokenUsage{PromptTokens: 1}); err == nil {
		t.Fatal("TokenUsage.Add accepted overflow")
	}
	if _, err := (TokenUsage{PromptTokens: math.MaxInt64, CompletionTokens: 1}).Total(); err == nil {
		t.Fatal("TokenUsage.Total accepted overflow")
	}
	left := ModelUsage{Model: "model", Calls: math.MaxInt, Cost: mustCost(t, 0)}
	right := ModelUsage{Model: "model", Calls: 1, Cost: mustCost(t, 0)}
	if _, err := left.Add(right); err == nil {
		t.Fatal("ModelUsage.Add accepted call-count overflow")
	}
}

func TestModelUsageSubtractOwnsRemainderInvariants(t *testing.T) {
	total := ModelUsage{
		Model: "model",
		TokenUsage: TokenUsage{
			PromptTokens: 10, CompletionTokens: 6, ReasoningTokens: 2,
			CacheReadTokens: 4, CacheWriteTokens: 3,
		},
		Cost: mustCost(t, 1), Calls: 3,
	}
	used := ModelUsage{
		Model: "model",
		TokenUsage: TokenUsage{
			PromptTokens: 4, CompletionTokens: 2, ReasoningTokens: 1,
			CacheReadTokens: 1, CacheWriteTokens: 1,
		},
		Cost: mustCost(t, 0.25), Calls: 1,
	}
	want := ModelUsage{
		Model: "model",
		TokenUsage: TokenUsage{
			PromptTokens: 6, CompletionTokens: 4, ReasoningTokens: 1,
			CacheReadTokens: 3, CacheWriteTokens: 2,
		},
		Cost: mustCost(t, 0.75), Calls: 2,
	}
	got, present, err := total.Subtract(used)
	if err != nil || !present || got != want {
		t.Fatalf("Subtract = (%+v, %t, %v), want (%+v, true, nil)", got, present, err, want)
	}
	if got, present, err := total.Subtract(total); err != nil || present || got != (ModelUsage{}) {
		t.Fatalf("exact Subtract = (%+v, %t, %v), want zero, false, nil", got, present, err)
	}

	for name, candidate := range map[string]ModelUsage{
		"different model": {Model: "other", Calls: 1},
		"token underflow": {
			Model: "model", TokenUsage: TokenUsage{PromptTokens: 11}, Cost: mustCost(t, 0), Calls: 1,
		},
		"cost underflow": {Model: "model", Cost: mustCost(t, 1.25), Calls: 1},
		"call underflow": {Model: "model", Cost: mustCost(t, 0), Calls: 4},
		"usage without remaining calls": {
			Model: "model", TokenUsage: TokenUsage{PromptTokens: 9}, Cost: mustCost(t, 1), Calls: 3,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := total.Subtract(candidate); err == nil {
				t.Fatal("Subtract accepted an invalid remainder")
			}
		})
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
			TokenUsage: TokenUsage{
				PromptTokens:     3,
				CompletionTokens: 2,
				ReasoningTokens:  1,
			},
			Cost:  mustCost(t, 0.25),
			Calls: 1,
		},
		{
			Model: "beta",
			TokenUsage: TokenUsage{
				PromptTokens:     5,
				CompletionTokens: 1,
			},
			Cost:  mustCost(t, 0.5),
			Calls: 2,
		},
	}}
	total, err := snapshot.Total()
	if err != nil {
		t.Fatalf("Total: %v", err)
	}
	if total.PromptTokens != 8 ||
		total.CompletionTokens != 3 ||
		total.ReasoningTokens != 1 ||
		total.Calls != 3 {
		t.Fatalf("total = %+v", total)
	}
	if cost, ok := total.Cost.USD(); !ok || cost != 0.75 {
		t.Fatalf("total cost = %g, %t; want 0.75, true", cost, ok)
	}

	overflow := Snapshot{Models: []ModelUsage{
		{Model: "alpha", TokenUsage: TokenUsage{PromptTokens: math.MaxInt64}, Calls: 1},
		{Model: "beta", TokenUsage: TokenUsage{PromptTokens: 1}, Calls: 1},
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

func TestSnapshotValidateAdvanceFromRejectsRegression(t *testing.T) {
	previous := Snapshot{Models: []ModelUsage{{
		Model: "model",
		TokenUsage: TokenUsage{
			PromptTokens: 4, CompletionTokens: 2, ReasoningTokens: 1,
			CacheReadTokens: 1, CacheWriteTokens: 1,
		},
		Cost:  mustCost(t, 0.5),
		Calls: 2,
	}}}
	next := previous
	next.Models = append([]ModelUsage(nil), previous.Models...)
	next.Models[0].Calls++
	if err := next.ValidateAdvanceFrom(previous); err != nil {
		t.Fatalf("ValidateAdvanceFrom: %v", err)
	}

	for name, mutate := range map[string]func(*Snapshot){
		"model removed": func(value *Snapshot) { value.Models = nil },
		"tokens":        func(value *Snapshot) { value.Models[0].PromptTokens-- },
		"cost": func(value *Snapshot) {
			value.Models[0].Cost = mustCost(t, 0.25)
		},
		"calls": func(value *Snapshot) { value.Models[0].Calls-- },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := previous
			candidate.Models = append([]ModelUsage(nil), previous.Models...)
			mutate(&candidate)
			if err := candidate.ValidateAdvanceFrom(previous); err == nil {
				t.Fatal("ValidateAdvanceFrom accepted cumulative usage regression")
			}
		})
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
