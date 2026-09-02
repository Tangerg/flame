package agent

import (
	"math"
	"strings"
	"testing"
	"time"

	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

func TestRunLifecycleShape(t *testing.T) {
	running := runningRun("seg_1")
	running.CreatedAt = time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC)
	running.ProtocolProfile = &runtimeprotocol.RunProtocolProfile{
		RequiredFeatures: []runtimeprotocol.RunProtocolFeature{runtimeprotocol.RunProtocolFeatureSubagents},
		InterruptTypes:   []runtimeprotocol.InterruptType{runtimeprotocol.InterruptApproval, runtimeprotocol.InterruptQuestion},
	}
	if err := running.Validate(); err != nil {
		t.Fatal(err)
	}
	waiting := running
	waiting.Status, waiting.ActiveSegmentID = runtimeprotocol.RunStatusWaiting, ""
	if err := waiting.Validate(); err != nil {
		t.Fatal(err)
	}
	finished := waiting
	finished.Status, finished.Outcome = runtimeprotocol.RunStatusFinished, Outcome{Status: OutcomeCompleted}
	finished.FinishedAt = finished.CreatedAt.Add(time.Second)
	if err := finished.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := running
	invalid.FinishedAt = invalid.CreatedAt.Add(time.Second)
	if err := invalid.Validate(); err == nil {
		t.Fatal("running run with a finish time was accepted")
	}
	cloned := running.Clone()
	cloned.ProtocolProfile.InterruptTypes[0] = runtimeprotocol.InterruptQuestion
	if running.ProtocolProfile.InterruptTypes[0] != runtimeprotocol.InterruptApproval || running.Equal(cloned) {
		t.Fatal("run clone shares its negotiated contract")
	}
	invalidContract := running.Clone()
	invalidContract.ProtocolProfile.RequiredFeatures = append(
		invalidContract.ProtocolProfile.RequiredFeatures,
		runtimeprotocol.RunProtocolFeatureSubagents,
	)
	if err := invalidContract.Validate(); err == nil {
		t.Fatal("run accepted a duplicate negotiated feature")
	}
}

func TestRunRejectsNonExactExecutionIdentity(t *testing.T) {
	run := runningRun("seg_1")
	run.ID = " run_1"
	if err := run.Validate(); err == nil {
		t.Fatal("Run accepted an identity that requires trimming")
	}
	run = runningRun(" seg_1")
	if err := run.Validate(); err == nil {
		t.Fatal("Run accepted an active segment identity that requires trimming")
	}
	run = runningRun("seg_1")
	run.ContextTokens = -1
	if err := run.Validate(); err == nil {
		t.Fatal("Run accepted negative context tokens")
	}
}

func TestRunLineageRequiresExplicitRootOrValidChild(t *testing.T) {
	t.Parallel()
	if err := (RunLineage{}).validate("run_1"); err == nil {
		t.Fatal("zero lineage was accepted as a root")
	}
	root := RootRunLineage()
	if err := root.validate("run_root"); err != nil || !root.IsRoot() {
		t.Fatalf("root lineage = (%+v, %v)", root, err)
	}
	child, err := NewChildRunLineage("run_child", "item_spawn", "run_parent", "run_root")
	if err != nil {
		t.Fatal(err)
	}
	if child.IsRoot() || child.SpawnedByBlockID() != "item_spawn" || child.ParentRunID() != "run_parent" || child.RootRunID() != "run_root" {
		t.Fatalf("child lineage = %+v", child)
	}
	for _, test := range []struct {
		name                 string
		runID, spawn, parent string
		root                 string
	}{
		{name: "missing spawn", runID: "run_child", parent: "run_parent", root: "run_root"},
		{name: "self parent", runID: "run_child", spawn: "item_spawn", parent: "run_child", root: "run_root"},
		{name: "self root", runID: "run_child", spawn: "item_spawn", parent: "run_parent", root: "run_child"},
		{name: "non-exact parent", runID: "run_child", spawn: "item_spawn", parent: " run_parent", root: "run_root"},
		{name: "non-exact root", runID: "run_child", spawn: "item_spawn", parent: "run_parent", root: "run_root "},
	} {
		if _, err := NewChildRunLineage(test.runID, test.spawn, test.parent, test.root); err == nil {
			t.Errorf("%s lineage was accepted", test.name)
		}
	}
}

func TestRunOptionsValidateBounds(t *testing.T) {
	temperature, topP, maxTokens := 0.7, 0.9, int64(4096)
	maxSteps, maxBudget := 20, float64(3)
	limits, err := NewRunLimits(RunLimitValues{MaxSteps: &maxSteps, MaxBudgetUSD: &maxBudget})
	if err != nil {
		t.Fatal(err)
	}
	options := RunOptions{
		Provider: "mock", Model: "balanced", ReasoningEffort: "high",
		Limits: limits, Generation: GenerationParams{
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
	options = RunOptions{ReasoningEffort: "high", Limits: UnlimitedRunLimits()}
	if err := options.Validate(); err == nil {
		t.Fatal("reasoning effort without a model was accepted")
	}
}

func TestRunOptionsEqualPreservesOptionalGenerationSemantics(t *testing.T) {
	zero := 0.0
	left := RunOptions{
		Provider: "deepseek", Model: "v4", Limits: UnlimitedRunLimits(),
		Generation: GenerationParams{Temperature: &zero, Stop: []string{"END"}},
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

func testRootRun(run Run) Run {
	run.Lineage = RootRunLineage()
	run.Limits = UnlimitedRunLimits()
	return run
}

func testChildRun(run Run) Run {
	run.Limits = UnlimitedRunLimits()
	return run
}

func testChildRunLineage(t *testing.T, runID, spawn, parent, root string) RunLineage {
	t.Helper()
	lineage, err := NewChildRunLineage(runID, spawn, parent, root)
	if err != nil {
		t.Fatal(err)
	}
	return lineage
}

func TestOutcomeValidationMatchesRuntimeUnion(t *testing.T) {
	problem := &runtimeprotocol.ProblemData{Type: "rate_limited", Detail: "deadline exceeded", RetryAfterSeconds: 2}
	valid := []Outcome{
		{Status: OutcomeCompleted},
		{Status: OutcomeTimedOut, Problem: problem},
		{Status: OutcomeFailed, Problem: &runtimeprotocol.ProblemData{Type: "provider_error", Detail: "provider failed"}},
		{Status: OutcomeLost, Problem: &runtimeprotocol.ProblemData{Type: "run_lost", Detail: "executor disappeared"}},
		{Status: OutcomeMaxSteps, Detail: "20 / 20 steps"},
		{Status: OutcomeMaxBudget, Detail: "$2.00 / $2.00"},
		{Status: OutcomeCanceled, Detail: "user stopped"},
	}
	for _, outcome := range valid {
		if err := outcome.Validate(); err != nil {
			t.Fatalf("valid outcome %+v: %v", outcome, err)
		}
	}
	for _, outcome := range []Outcome{
		{Status: OutcomeTimedOut},
		{Status: OutcomeFailed, Detail: "wrong channel"},
		{Status: OutcomeCanceled, Problem: problem},
		{Status: OutcomeCompleted, Detail: "unexpected"},
		{Status: OutcomeCompleted, Problem: problem},
		{Status: OutcomeFailed, Problem: &runtimeprotocol.ProblemData{}},
	} {
		if err := outcome.Validate(); err == nil {
			t.Fatalf("invalid outcome %+v was accepted", outcome)
		}
	}
	cloned := valid[1].Clone()
	cloned.Problem.Detail = "mutated"
	if valid[1].Equal(cloned) || !valid[1].Equal(valid[1].Clone()) {
		t.Fatal("outcome problem is not value-owned")
	}
}

func TestOutcomeExplanationIncludesRecoveryMetadata(t *testing.T) {
	outcome := Outcome{Status: OutcomeFailed, Problem: &runtimeprotocol.ProblemData{
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
		ByModel: map[string]ModelUsage{"deepseek/v4": {InputTokens: 12, CostUSD: &modelCost}},
	}
	if err := usage.Validate(); err != nil {
		t.Fatal(err)
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

	invalid := math.NaN()
	if err := (Usage{CostUSD: &invalid}).Validate(); err == nil {
		t.Fatal("NaN cost was accepted")
	}
	if err := validateUsageProgress(Usage{CostUSD: &knownZero}, Usage{}); err != nil {
		t.Fatalf("known cumulative cost could not become unknown: %v", err)
	}
	regressedCost, priorCost := 0.25, 0.5
	if err := validateUsageProgress(Usage{CostUSD: &priorCost}, Usage{CostUSD: &regressedCost}); err == nil {
		t.Fatal("known cumulative cost regressed")
	}
	if err := (Usage{Steps: -1}).Validate(); err == nil {
		t.Fatal("negative step usage was accepted")
	}
	if err := (Usage{ByModel: map[string]ModelUsage{"": {}}}).Validate(); err == nil {
		t.Fatal("empty model attribution key was accepted")
	}
	if err := (Usage{ByModel: map[string]ModelUsage{"bad model": {}}}).Validate(); err == nil {
		t.Fatal("non-canonical model attribution key was accepted")
	}
	if err := (Usage{ByModel: map[string]ModelUsage{
		strings.Repeat("m", runtimeprotocol.MaximumModelIdentityCharacters+1): {},
	}}).Validate(); err == nil {
		t.Fatal("overlong model attribution key was accepted")
	}
	if err := validateUsageProgress(
		Usage{Steps: 3, ByModel: map[string]ModelUsage{"deepseek/v4": {InputTokens: 12}}},
		Usage{Steps: 2},
	); err == nil {
		t.Fatal("step or per-model usage regression was accepted")
	}
}
