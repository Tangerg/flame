package agent

import (
	"strings"
	"testing"

	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

func TestRunOptionsValidateBounds(t *testing.T) {
	temperature, topP, maxTokens := 0.7, 0.9, int64(4096)
	maxSteps, maxBudget := 20, float64(3)
	limits, err := NewRunLimits(RunLimitValues{MaxSteps: &maxSteps, MaxBudgetUSD: &maxBudget})
	if err != nil {
		t.Fatal(err)
	}
	options := RunOptions{
		Provider: "mock", Model: "balanced", ReasoningEffort: "high",
		Limits: limits, Generation: runtimeprotocol.GenerationParams{
			Temperature: &temperature, TopP: &topP, MaxTokens: &maxTokens, Stop: []string{"END"},
		},
	}
	if err := options.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := 3.0
	options.Generation.Temperature = &bad
	if err := options.Validate(); err == nil {
		t.Fatal("invalid temperature was accepted")
	}
	options.Generation = runtimeprotocol.GenerationParams{Stop: []string{"END", "END"}}
	if err := options.Validate(); err == nil {
		t.Fatal("duplicate stop sequences were accepted")
	}
	options = RunOptions{ReasoningEffort: "high", Limits: UnlimitedRunLimits()}
	if err := options.Validate(); err == nil {
		t.Fatal("reasoning effort without a model was accepted")
	}
}

func TestRunOptionsEqualPreservesOptionalGenerationSemantics(t *testing.T) {
	zero := 0.0
	left := RunOptions{
		Provider: "deepseek", Model: "v4", Limits: UnlimitedRunLimits(),
		Generation: runtimeprotocol.GenerationParams{Temperature: &zero, Stop: []string{"END"}},
	}
	if !left.Equal(left.Clone()) {
		t.Fatal("cloned options are not equal")
	}
	right := left.Clone()
	right.Generation.Temperature = nil
	if left.Equal(right) {
		t.Fatal("explicit zero temperature equals an omitted temperature")
	}
	right = left.Clone()
	right.Generation.Stop[0] = "STOP"
	if left.Equal(right) {
		t.Fatal("different stop sequences are equal")
	}
	right = left.Clone()
	right.ReasoningEffort = "high"
	if left.Equal(right) {
		t.Fatal("different reasoning effort is equal")
	}
}

func TestOutcomeExplanationIncludesRecoveryMetadata(t *testing.T) {
	outcome := Outcome{Status: runtimeprotocol.OutcomeFailed, Problem: &runtimeprotocol.ProblemData{
		Type: "rate_limited", Detail: "quota exhausted", RetryAfterSeconds: 12,
	}}
	if got := outcome.Description(); got != "quota exhausted" {
		t.Fatalf("Description = %q, want concise detail", got)
	}
	if got := outcome.Explanation(); !strings.Contains(got, "quota exhausted") || !strings.Contains(got, "retry after 12s") {
		t.Fatalf("Explanation = %q, want recovery metadata", got)
	}
}

func TestUsagePreservesOptionalCostSemantics(t *testing.T) {
	knownZero, modelCost := 0.0, 0.25
	usage := Usage{
		CostUSD: &knownZero, Steps: 3,
		ByModel: map[string]runtimeprotocol.ModelUsage{"deepseek/v4": {InputTokens: 12, CostUSD: &modelCost}},
	}
	cloned := usage.Clone()
	*usage.CostUSD = 1
	model := usage.ByModel["deepseek/v4"]
	*model.CostUSD = 2
	usage.ByModel["deepseek/v4"] = model
	if cloned.CostUSD == nil || *cloned.CostUSD != 0 || cloned.ByModel["deepseek/v4"].CostUSD == nil ||
		*cloned.ByModel["deepseek/v4"].CostUSD != 0.25 || !cloned.Equal(cloned.Clone()) || cloned.Empty() {
		t.Fatalf("cloned usage = %+v", cloned)
	}

	if err := validateUsageProgress(Usage{CostUSD: &knownZero}, Usage{}); err != nil {
		t.Fatalf("known cumulative cost could not become unknown: %v", err)
	}
	regressedCost, priorCost := 0.25, 0.5
	if err := validateUsageProgress(Usage{CostUSD: &priorCost}, Usage{CostUSD: &regressedCost}); err == nil {
		t.Fatal("known cumulative cost regressed")
	}
	if err := validateUsageProgress(
		Usage{Steps: 3, ByModel: map[string]runtimeprotocol.ModelUsage{"deepseek/v4": {InputTokens: 12}}},
		Usage{Steps: 2},
	); err == nil {
		t.Fatal("step or per-model usage regression was accepted")
	}
}
