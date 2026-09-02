package sessions

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
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

// UsageReport is one session's cumulative metering and per-model split.
type UsageReport struct {
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

// NewUsageReporter constructs a complete usage reporter over the supplied projections.
func NewUsageReporter(deps UsageDependencies) (*UsageReporter, error) {
	if nilDependency(deps.Runs) {
		return nil, errors.New("sessions: usage Run reader is required")
	}
	if nilDependency(deps.Sessions) {
		return nil, errors.New("sessions: usage session lister is required")
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &UsageReporter{
		runs: deps.Runs, sessions: deps.Sessions, now: now,
	}, nil
}

// Session returns one session's cumulative metering and per-model split.
func (r *UsageReporter) Session(ctx context.Context, sessionID string) (UsageReport, error) {
	runs, err := r.runs.ListRuns(ctx, sessionID)
	if err != nil {
		return UsageReport{}, err
	}
	total := usageAccumulator{}
	byModel := map[string]*usageAccumulator{}
	for _, current := range runs {
		if err := foldRun(current, time.Time{}, &total, nil, byModel, nil, false); err != nil {
			return UsageReport{}, err
		}
	}
	report := UsageReport{Total: total.usage()}
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
		for _, current := range runs {
			if err := foldRun(current, since, &total, byProvider, byModel, byDay, true); err != nil {
				return UsageSummary{}, err
			}
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

func foldRun(current run.Run, since time.Time, total *usageAccumulator, byProvider, byModel, byDay map[string]*usageAccumulator, qualifyModels bool) error {
	usage, reported := current.Metrics().Usage()
	if !current.State().IsTerminal() || !reported {
		return nil
	}
	if !since.IsZero() && !current.FinishedAt().IsZero() && current.FinishedAt().Before(since) {
		return nil
	}
	if total != nil {
		if err := total.addRun(usage.Total); err != nil {
			return fmt.Errorf("sessions: aggregate Run %q usage: %w", current.ID(), err)
		}
	}
	if byProvider != nil {
		bucket := accumulatorFor(byProvider, current.ModelSelection().Provider())
		if err := bucket.addRun(usage.Total); err != nil {
			return fmt.Errorf("sessions: aggregate provider %q usage: %w", current.ModelSelection().Provider(), err)
		}
	}
	if byDay != nil && !current.FinishedAt().IsZero() {
		day := current.FinishedAt().UTC().Format(time.DateOnly)
		bucket := accumulatorFor(byDay, day)
		if err := bucket.addRun(usage.Total); err != nil {
			return fmt.Errorf("sessions: aggregate day %q usage: %w", day, err)
		}
	}
	if byModel == nil {
		return nil
	}
	if len(usage.ByModel) > 0 {
		for name, split := range usage.ByModel {
			key := usageModelKey(current.ModelSelection().Provider(), name, qualifyModels)
			bucket := accumulatorFor(byModel, key)
			if err := bucket.addRun(split); err != nil {
				return fmt.Errorf("sessions: aggregate model %q usage: %w", key, err)
			}
		}
		return nil
	}
	key := usageModelKey(current.ModelSelection().Provider(), current.ModelSelection().Model(), qualifyModels)
	bucket := accumulatorFor(byModel, key)
	if err := bucket.addRun(usage.Total); err != nil {
		return fmt.Errorf("sessions: aggregate model %q usage: %w", key, err)
	}
	return nil
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
	tokens       accounting.Totals
	cost         accounting.Cost
	costObserved bool
	runs         int
}

func (u *usageAccumulator) addRun(usage accounting.Totals) error {
	if u.runs < 0 || (!u.costObserved && u.cost != (accounting.Cost{})) {
		return fmt.Errorf("usage accumulator is invalid")
	}
	if err := u.tokens.Validate(); err != nil {
		return fmt.Errorf("usage accumulator: %w", err)
	}
	if err := u.cost.Validate(); err != nil {
		return fmt.Errorf("usage accumulator: %w", err)
	}
	if err := usage.Validate(); err != nil {
		return err
	}
	if u.runs == math.MaxInt {
		return fmt.Errorf("usage Run count overflows")
	}
	next := *u
	fields := []struct {
		name  string
		value *int64
		add   int64
	}{
		{name: "input tokens", value: &next.tokens.InputTokens, add: usage.InputTokens},
		{name: "output tokens", value: &next.tokens.OutputTokens, add: usage.OutputTokens},
		{name: "cache-read tokens", value: &next.tokens.CacheReadTokens, add: usage.CacheReadTokens},
		{name: "cache-write tokens", value: &next.tokens.CacheWriteTokens, add: usage.CacheWriteTokens},
		{name: "reasoning tokens", value: &next.tokens.ReasoningTokens, add: usage.ReasoningTokens},
	}
	for _, field := range fields {
		if field.add > math.MaxInt64-*field.value {
			return fmt.Errorf("usage %s overflow", field.name)
		}
		*field.value += field.add
	}
	cost, err := accounting.CostFromOptional(usage.CostUSD)
	if err != nil {
		return err
	}
	if next.costObserved {
		next.cost, err = next.cost.Add(cost)
		if err != nil {
			return err
		}
	} else {
		next.cost = cost
		next.costObserved = true
	}
	next.runs++
	*u = next
	return nil
}

func (u usageAccumulator) usage() accounting.Totals {
	out := u.tokens
	if u.costObserved {
		out.CostUSD = u.cost.OptionalUSD()
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
