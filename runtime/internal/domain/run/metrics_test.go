package run

import (
	"fmt"
	"math"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/core/chat"
)

// TestMetricsCostAnswersForAnUnaccountedRun keeps the answer a caller branches
// on with the value that holds it: a Run that has not reported usage costs
// zero rather than failing.
func TestMetricsCostAnswersForAnUnaccountedRun(t *testing.T) {
	t.Parallel()

	var unaccounted Metrics
	cost, err := unaccounted.Cost()
	if err != nil {
		t.Fatalf("Cost of an unaccounted Run = %v", err)
	}
	if usd, available := cost.USD(); available || usd != 0 {
		t.Fatalf("Cost of an unaccounted Run = (%v, %t), want an unavailable zero", usd, available)
	}
}

func TestRecoveredCallPreservesUnreportedUsageAndRejectsOverflow(t *testing.T) {
	metrics, err := NewMetrics(nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	next, err := metrics.AccrueModelCall("served-model", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, reported := next.Usage(); reported || next.Steps() != 1 {
		t.Fatal("unreported source became zero usage")
	}
	usage := accounting.Usage{Total: accounting.Totals{Tokens: accounting.Tokens{InputTokens: math.MaxInt64}}}
	metrics, err = NewMetrics(&usage, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := metrics.AccrueModelCall("served-model", &chat.Usage{InputTokens: 1}, nil); err == nil {
		t.Fatal("recovered usage overflowed silently")
	}
	if value, _ := metrics.Usage(); value.Total.InputTokens != math.MaxInt64 {
		t.Fatal("rejected recovery mutated prior metrics")
	}
}

func TestModelCallAccountingKeepsUnknownPriceAcrossCallOrder(t *testing.T) {
	for _, unknownFirst := range []bool{true, false} {
		t.Run(fmt.Sprint(unknownFirst), func(t *testing.T) {
			var current Metrics
			price := 0.25
			accrue := func(model string, usage *chat.Usage, cost *float64) {
				t.Helper()
				next, err := current.AccrueModelCall(model, usage, cost)
				if err != nil {
					t.Fatal(err)
				}
				if err := next.ValidateAdvanceFrom(current); err != nil {
					t.Fatal(err)
				}
				current = next
			}
			if unknownFirst {
				accrue("unknown-model", nil, nil)
			}
			accrue("known-model", &chat.Usage{InputTokens: 7, OutputTokens: 2}, &price)
			if !unknownFirst {
				accrue("unknown-model", nil, nil)
			}
			accrue("unknown-model", &chat.Usage{InputTokens: 3, OutputTokens: 1}, &price)
			usage, reported := current.Usage()
			if !reported || current.Steps() != 3 || usage.Total.InputTokens != 10 || usage.Total.OutputTokens != 3 || usage.Total.CostUSD != nil {
				t.Fatalf("consumption = %+v, steps=%d", usage, current.Steps())
			}
			if unknown, present := usage.ByModel["unknown-model"]; present && unknown.CostUSD != nil {
				t.Fatalf("later known response erased original unknown price: %+v", unknown)
			}
		})
	}
}
