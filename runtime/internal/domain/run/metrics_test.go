package run

import "testing"

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
