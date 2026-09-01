package sessions

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type usageRunReaderStub struct{ runs []run.Run }

func (s usageRunReaderStub) ListRuns(context.Context, string) ([]run.Run, error) {
	return s.runs, nil
}

func TestSummaryPeriodSeparatesAllTimeFromRecentDays(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 30, 0, 0, time.FixedZone("test", 8*60*60))
	if since, err := AllTimeUsage().Since(now); err != nil || !since.IsZero() {
		t.Fatalf("all-time Since = (%v, %v), want zero", since, err)
	}
	recent, err := RecentUsageDays(7)
	if err != nil {
		t.Fatal(err)
	}
	want := now.UTC().AddDate(0, 0, -7)
	if since, err := recent.Since(now); err != nil || !since.Equal(want) {
		t.Fatalf("recent Since = (%v, %v), want %v", since, err, want)
	}
	for _, days := range []int{0, -1} {
		if _, err := RecentUsageDays(days); !errors.Is(err, ErrInvalidUsageSummaryPeriod) {
			t.Fatalf("RecentDays(%d) = %v", days, err)
		}
	}
	if _, err := (UsageSummaryPeriod{days: 1}).Since(now); !errors.Is(err, ErrInvalidUsageSummaryPeriod) {
		t.Fatalf("corrupt all-time period = %v", err)
	}
}

func usd(v float64) *float64 { return &v }

func mustUsageCost(t *testing.T, usd float64) accounting.Cost {
	t.Helper()
	cost, err := accounting.NewCost(usd)
	if err != nil {
		t.Fatalf("NewCost(%g): %v", usd, err)
	}
	return cost
}

func finishedRun(t *testing.T, provider, model string, at time.Time, usage accounting.Usage) run.Run {
	t.Helper()
	return testsupport.MustRestoreRun(run.Snapshot{ID: "run_x", ModelSelection: mustUsageSelection(t, provider, model), State: run.Completed,
		FinishedAt: at, Metrics: testsupport.MustRunMetrics(testsupport.RunMetricsInput{Usage: &usage})})

}

func mustUsageSelection(t testing.TB, provider, model string) modelref.Selection {
	t.Helper()
	selection, err := modelref.New(provider, model)
	if err != nil {
		t.Fatalf("modelref.New(%q, %q): %v", provider, model, err)
	}
	return selection
}

func TestFoldRunFoldsAllDimensions(t *testing.T) {
	day := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	run := finishedRun(t, "anthropic", "claude-opus-4-8", day, accounting.Usage{
		Total: accounting.Totals{InputTokens: 100, OutputTokens: 40, CostUSD: usd(1.5)},
	})

	total := usageAccumulator{}
	byProvider := map[string]*usageAccumulator{}
	byModel := map[string]*usageAccumulator{}
	byDay := map[string]*usageAccumulator{}
	if err := foldRun(run, time.Time{}, &total, byProvider, byModel, byDay, false); err != nil {
		t.Fatal(err)
	}

	cost, costAvailable := total.cost.USD()
	if total.runs != 1 || total.tokens.InputTokens != 100 || !costAvailable || cost != 1.5 {
		t.Fatalf("total = %+v", total)
	}
	if byProvider["anthropic"] == nil || byProvider["anthropic"].tokens.OutputTokens != 40 {
		t.Errorf("byProvider missing anthropic: %+v", byProvider)
	}
	if byModel["claude-opus-4-8"] == nil {
		t.Errorf("byModel missing model: %+v", byModel)
	}
	if byDay["2026-06-21"] == nil {
		t.Errorf("byDay missing 2026-06-21: %+v", byDay)
	}
}

func TestFoldRunPrefersByModelSplit(t *testing.T) {
	run := finishedRun(t, "anthropic", "claude-opus-4-8", time.Now().UTC(), accounting.Usage{
		Total: accounting.Totals{InputTokens: 120, CostUSD: usd(2)},
		ByModel: map[string]accounting.Totals{
			"claude-opus-4-8":  {InputTokens: 100, CostUSD: usd(1.8)},
			"claude-haiku-4-5": {InputTokens: 20, CostUSD: usd(0.2)},
		},
	})
	byModel := map[string]*usageAccumulator{}
	if err := foldRun(run, time.Time{}, nil, nil, byModel, nil, false); err != nil {
		t.Fatal(err)
	}

	if len(byModel) != 2 {
		t.Fatalf("expected 2 model buckets, got %+v", byModel)
	}
	if byModel["claude-haiku-4-5"] == nil || byModel["claude-haiku-4-5"].tokens.InputTokens != 20 {
		t.Errorf("utility model not split out: %+v", byModel)
	}
}

func TestFoldRunSkipsUnfinishedAndOld(t *testing.T) {
	total := usageAccumulator{}

	if err := foldRun(testsupport.MustRestoreRun(run.Snapshot{State: run.Running}), time.Time{}, &total, nil, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	noUsage := testsupport.MustRestoreRun(run.Snapshot{State: run.Completed})
	if err := foldRun(noUsage, time.Time{}, &total, nil, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	old := finishedRun(t, "anthropic", "m", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		accounting.Usage{Total: accounting.Totals{InputTokens: 99}})
	if err := foldRun(old, time.Now().UTC().AddDate(0, 0, -1), &total, nil, nil, nil, false); err != nil {
		t.Fatal(err)
	}

	if total.runs != 0 {
		t.Errorf("expected nothing folded, got runs=%d tokens=%d", total.runs, total.tokens.InputTokens)
	}
}

func TestFoldRunQualifiesSummaryModelsWithTheirProvider(t *testing.T) {
	current := finishedRun(t, "openai-compatible", "shared-model", time.Now().UTC(), accounting.Usage{
		Total: accounting.Totals{InputTokens: 10},
		ByModel: map[string]accounting.Totals{
			"shared-model": {InputTokens: 10},
		},
	})
	byModel := map[string]*usageAccumulator{}
	if err := foldRun(current, time.Time{}, nil, nil, byModel, nil, true); err != nil {
		t.Fatal(err)
	}
	if byModel["openai-compatible/shared-model"] == nil || byModel["shared-model"] != nil {
		t.Fatalf("summary model buckets = %+v", byModel)
	}
}

func TestAccumulatorOmitsCostWhenUnpriced(t *testing.T) {
	a := usageAccumulator{}
	if err := a.addRun(accounting.Totals{InputTokens: 10}); err != nil {
		t.Fatal(err)
	}
	if got := a.usage(); got.CostUSD != nil {
		t.Errorf("CostUSD = %v, want nil", *got.CostUSD)
	}
	if err := a.addRun(accounting.Totals{InputTokens: 5, CostUSD: usd(0.3)}); err != nil {
		t.Fatal(err)
	}
	if got := a.usage(); got.CostUSD != nil {
		t.Errorf("CostUSD = %v, want nil after an unpriced component", *got.CostUSD)
	}

	priced := usageAccumulator{}
	if err := priced.addRun(accounting.Totals{CostUSD: usd(0)}); err != nil {
		t.Fatal(err)
	}
	if err := priced.addRun(accounting.Totals{CostUSD: usd(0.3)}); err != nil {
		t.Fatal(err)
	}
	if got := priced.usage(); got.CostUSD == nil || *got.CostUSD != 0.3 {
		t.Errorf("priced CostUSD = %v, want 0.3", got.CostUSD)
	}
}

func TestSessionUsageDoesNotPresentKnownPartialCostAsTotal(t *testing.T) {
	now := time.Now().UTC()
	reporter := NewUsageReporter(UsageDependencies{Runs: usageRunReaderStub{runs: []run.Run{
		finishedRun(t, "private", "served-alias", now, accounting.Usage{
			Total: accounting.Totals{InputTokens: 10},
		}),
		finishedRun(t, "private", "served-alias", now.Add(time.Second), accounting.Usage{
			Total: accounting.Totals{InputTokens: 5, CostUSD: usd(0.3)},
		}),
	}}})
	report, err := reporter.Session(t.Context(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.Total.InputTokens != 15 || report.Total.CostUSD != nil {
		t.Fatalf("Session usage = %+v, want 15 input tokens with unknown total cost", report.Total)
	}
	model := report.ByModel["served-alias"]
	if model.InputTokens != 15 || model.CostUSD != nil {
		t.Fatalf("model usage = %+v, want 15 input tokens with unknown total cost", model)
	}
}

func TestBucketsBySpendRanksByCostDesc(t *testing.T) {
	m := map[string]*usageAccumulator{
		"cheap": {tokens: accounting.Totals{InputTokens: 1}, cost: mustUsageCost(t, 0.1), costObserved: true},
		"dear":  {tokens: accounting.Totals{InputTokens: 1}, cost: mustUsageCost(t, 9), costObserved: true},
	}
	out := bucketsBySpend(m)
	if out[0].Key != "dear" {
		t.Errorf("expected dear first (spend-ranked), got %+v", out)
	}
}

func TestAccumulatorRejectsOverflowWithoutMutation(t *testing.T) {
	a := usageAccumulator{}
	if err := a.addRun(accounting.Totals{InputTokens: math.MaxInt64, CostUSD: usd(math.MaxFloat64)}); err != nil {
		t.Fatal(err)
	}
	before := a
	if err := a.addRun(accounting.Totals{InputTokens: 1, CostUSD: usd(math.MaxFloat64)}); err == nil {
		t.Fatal("overflowing aggregate was accepted")
	}
	if a != before {
		t.Fatalf("failed aggregation mutated accumulator: before=%+v after=%+v", before, a)
	}
}
