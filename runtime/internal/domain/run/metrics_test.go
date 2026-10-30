package run

import (
	"fmt"
	"math"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/core/chat"
)

func TestMetricsCostSeparatesNoDispatchFromUnreportedUsage(t *testing.T) {
	t.Parallel()

	var idle Metrics
	cost, err := idle.Cost()
	if usd, available := cost.USD(); err != nil || !available || usd != 0 {
		t.Fatalf("Cost of a Run without dispatched calls = (%v, %t, %v), want an exact zero", usd, available, err)
	}
	unreported, err := idle.AccrueModelCall("served-model", nil, accounting.Cost{})
	if err != nil {
		t.Fatal(err)
	}
	cost, err = unreported.Cost()
	if usd, available := cost.USD(); err != nil || available || usd != 0 {
		t.Fatalf("Cost of a dispatched call without usage = (%v, %t, %v), want unpriced", usd, available, err)
	}
}

func TestRecoveredCallPreservesUnreportedUsageAndRejectsOverflow(t *testing.T) {
	metrics, err := NewMetrics(nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	next, err := metrics.AccrueModelCall("served-model", nil, accounting.Cost{})
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
	if _, err := metrics.AccrueModelCall("served-model", &chat.Usage{InputTokens: 1}, accounting.Cost{}); err == nil {
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
			price, err := accounting.NewCost(0.25)
			if err != nil {
				t.Fatal(err)
			}
			accrue := func(model string, usage *chat.Usage, cost accounting.Cost) {
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
				accrue("unknown-model", nil, accounting.Cost{})
			}
			accrue("known-model", &chat.Usage{InputTokens: 7, OutputTokens: 2}, price)
			if !unknownFirst {
				accrue("unknown-model", nil, accounting.Cost{})
			}
			accrue("unknown-model", &chat.Usage{InputTokens: 3, OutputTokens: 1}, price)
			usage, reported := current.Usage()
			if !reported || current.Steps() != 3 || usage.Total.InputTokens != 10 || usage.Total.OutputTokens != 3 || usage.Total.Cost != (accounting.Cost{}) {
				t.Fatalf("consumption = %+v, steps=%d", usage, current.Steps())
			}
			if unknown, present := usage.ByModel["unknown-model"]; present && unknown.Cost != (accounting.Cost{}) {
				t.Fatalf("later known response erased original unknown price: %+v", unknown)
			}
			if known := usage.ByModel["known-model"]; known.InputTokens != 7 || known.OutputTokens != 2 {
				t.Fatalf("unreported calls erased a known model breakdown: %+v", usage.ByModel)
			}
		})
	}
}
