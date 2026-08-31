package sessions

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// UsageRunReader reads the durable run history for one session.
type UsageRunReader interface {
	ListRuns(ctx context.Context, sessionID string) ([]run.Run, error)
}

// UsageSessionLister lists the user-facing sessions that contribute to aggregate
// usage. Child sessions are excluded by the session use case, preventing
// subtree-aggregated runs from being counted twice.
type UsageSessionLister interface {
	List(ctx context.Context) ([]session.Session, error)
}

// UsageBucket is one named portion of a summary report.
type UsageBucket struct {
	Key   string
	Usage accounting.Totals
	Runs  int
}

// SessionUsageReport is one session's cumulative metering and per-model split.
type SessionUsageReport struct {
	Total   accounting.Totals
	ByModel map[string]accounting.Totals
}

// UsageSummary is a cross-session usage report. Provider and day buckets reconcile
// with Total because every completed run contributes as one whole run.
type UsageSummary struct {
	Total      accounting.Totals
	ByProvider []UsageBucket
	ByModel    []UsageBucket
	ByDay      []UsageBucket
	Sessions   int
	Runs       int
}

// UsageDependencies are the durable projections and model policy a UsageReporter needs.
type UsageDependencies struct {
	Runs     UsageRunReader
	Sessions UsageSessionLister
	Now      func() time.Time
}

// UsageReporter folds durable terminal run records into read-only usage reports.
type UsageReporter struct {
	runs     UsageRunReader
	sessions UsageSessionLister
	now      func() time.Time
}

// NewUsageReporter constructs a usage reporter over the supplied projections.
func NewUsageReporter(deps UsageDependencies) *UsageReporter {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &UsageReporter{
		runs: deps.Runs, sessions: deps.Sessions, now: now,
	}
}

// Session returns one session's cumulative metering and per-model split.
func (r *UsageReporter) Session(ctx context.Context, sessionID string) (SessionUsageReport, error) {
	runs, err := r.runs.ListRuns(ctx, sessionID)
	if err != nil {
		return SessionUsageReport{}, err
	}
	total := usageAccumulator{}
	byModel := map[string]*usageAccumulator{}
	for _, run := range runs {
		foldRun(run, time.Time{}, &total, nil, byModel, nil, false)
	}
	report := SessionUsageReport{Total: total.usage()}
	if len(byModel) > 0 {
		report.ByModel = make(map[string]accounting.Totals, len(byModel))
		for name, bucket := range byModel {
			report.ByModel[name] = bucket.usage()
		}
	}
	return report, nil
}

// Summary returns usage across user-facing sessions under the requested
// all-time or recent calendar-day period.
func (r *UsageReporter) Summary(ctx context.Context, period UsageSummaryPeriod) (UsageSummary, error) {
	since, err := period.Since(r.now())
	if err != nil {
		return UsageSummary{}, err
	}
	sessions, err := r.sessions.List(ctx)
	if err != nil {
		return UsageSummary{}, err
	}

	total := usageAccumulator{}
	byProvider := map[string]*usageAccumulator{}
	byModel := map[string]*usageAccumulator{}
	byDay := map[string]*usageAccumulator{}
	sessionCount := 0
	for _, sess := range sessions {
		runs, err := r.runs.ListRuns(ctx, sess.ID())
		if err != nil {
			return UsageSummary{}, err
		}
		before := total.runs
		for _, run := range runs {
			foldRun(run, since, &total, byProvider, byModel, byDay, true)
		}
		if total.runs > before {
			sessionCount++
		}
	}

	return UsageSummary{
		Total:      total.usage(),
		ByProvider: bucketsBySpend(byProvider),
		ByModel:    bucketsBySpend(byModel),
		ByDay:      bucketsByKey(byDay),
		Sessions:   sessionCount,
		Runs:       total.runs,
	}, nil
}

func foldRun(current run.Run, since time.Time, total *usageAccumulator, byProvider, byModel, byDay map[string]*usageAccumulator, qualifyModels bool) {
	usage, reported := current.Metrics().Usage()
	if !current.State().IsTerminal() || !reported {
		return
	}
	if !since.IsZero() && !current.FinishedAt().IsZero() && current.FinishedAt().Before(since) {
		return
	}
	if total != nil {
		total.add(usage.Total)
		total.runs++
	}
	if byProvider != nil {
		bucket := accumulatorFor(byProvider, current.ModelSelection().Provider())
		bucket.add(usage.Total)
		bucket.runs++
	}
	if byDay != nil && !current.FinishedAt().IsZero() {
		bucket := accumulatorFor(byDay, current.FinishedAt().UTC().Format(time.DateOnly))
		bucket.add(usage.Total)
		bucket.runs++
	}
	if byModel == nil {
		return
	}
	if len(usage.ByModel) > 0 {
		for name, split := range usage.ByModel {
			bucket := accumulatorFor(byModel, usageModelKey(current.ModelSelection().Provider(), name, qualifyModels))
			bucket.add(split)
			bucket.runs++
		}
		return
	}
	bucket := accumulatorFor(byModel, usageModelKey(current.ModelSelection().Provider(), current.ModelSelection().Model(), qualifyModels))
	bucket.add(usage.Total)
	bucket.runs++
}

func usageModelKey(provider, model string, qualified bool) string {
	if !qualified || provider == "" {
		return model
	}
	return provider + "/" + model
}

// usageAccumulator preserves the metering fields needed while folding Run
// records into one report bucket.
type usageAccumulator struct {
	tokens  accounting.Totals
	cost    float64
	hasCost bool
	runs    int
}

func (u *usageAccumulator) add(usage accounting.Totals) {
	u.tokens.InputTokens += usage.InputTokens
	u.tokens.OutputTokens += usage.OutputTokens
	u.tokens.CacheReadTokens += usage.CacheReadTokens
	u.tokens.CacheWriteTokens += usage.CacheWriteTokens
	u.tokens.ReasoningTokens += usage.ReasoningTokens
	if usage.CostUSD != nil {
		u.cost += *usage.CostUSD
		u.hasCost = true
	}
}

func (u usageAccumulator) usage() accounting.Totals {
	out := u.tokens
	if u.hasCost {
		cost := u.cost
		out.CostUSD = &cost
	}
	return out
}

func accumulatorFor(byKey map[string]*usageAccumulator, key string) *usageAccumulator {
	bucket := byKey[key]
	if bucket == nil {
		bucket = &usageAccumulator{}
		byKey[key] = bucket
	}
	return bucket
}

func bucketsBySpend(byKey map[string]*usageAccumulator) []UsageBucket {
	buckets := bucketsOf(byKey)
	slices.SortFunc(buckets, func(a, b UsageBucket) int {
		return cmp.Or(
			cmp.Compare(costOf(b.Usage.CostUSD), costOf(a.Usage.CostUSD)),
			cmp.Compare(b.Usage.InputTokens, a.Usage.InputTokens),
		)
	})
	return buckets
}

func bucketsByKey(byKey map[string]*usageAccumulator) []UsageBucket {
	buckets := bucketsOf(byKey)
	slices.SortFunc(buckets, func(a, b UsageBucket) int { return cmp.Compare(a.Key, b.Key) })
	return buckets
}

func bucketsOf(byKey map[string]*usageAccumulator) []UsageBucket {
	buckets := make([]UsageBucket, 0, len(byKey))
	for key, accumulated := range byKey {
		buckets = append(buckets, UsageBucket{Key: key, Usage: accumulated.usage(), Runs: accumulated.runs})
	}
	return buckets
}

func costOf(cost *float64) float64 {
	if cost == nil {
		return 0
	}
	return *cost
}
