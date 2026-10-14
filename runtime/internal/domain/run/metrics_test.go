package run

import (
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/core/chat"
	"math"
	"testing"
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
