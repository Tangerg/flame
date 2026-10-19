package sessions

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	modelchoice "github.com/Tangerg/flame/runtime/model"
)

// UsageSource reads retained physical evidence from one snapshot. The callback
// must not perform storage I/O while that snapshot owns the database connection.
type UsageSource interface {
	VisitEndpointInvocations(context.Context, string, time.Time, func(modelref.EndpointObservation) error) error
}

type UsageCoverage struct {
	Calls          int
	ReportedCalls  int
	PricedCalls    int
	StartedCalls   int
	CompletedCalls int
	FailedCalls    int
	UnknownCalls   int
}

type UsageBucket struct {
	Key             string
	Usage           accounting.Totals
	RecordedCostUSD *float64
	Coverage        UsageCoverage
}

type UsageReport struct {
	Total           accounting.Totals
	RecordedCostUSD *float64
	Coverage        UsageCoverage
	ByModel         []UsageBucket
}

type UsageSummary struct {
	UsageReport
	ObservedAt time.Time
	Since      time.Time
	ByProvider []UsageBucket
	ByDay      []UsageBucket
	ByOrigin   []UsageBucket
	ByScope    []UsageBucket
	Sessions   int
	Runs       int
}

type UsageDependencies struct {
	Source UsageSource
	Now    func() time.Time
}

type UsageReporter struct {
	source UsageSource
	now    func() time.Time
}

func NewUsageReporter(deps UsageDependencies) (*UsageReporter, error) {
	if dependency.Missing(deps.Source) {
		return nil, errors.New("sessions: physical usage source is required")
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &UsageReporter{source: deps.Source, now: now}, nil
}

func (r *UsageReporter) Session(ctx context.Context, sessionID string) (UsageReport, error) {
	if err := resourceid.ValidateSession(sessionID); err != nil {
		return UsageReport{}, err
	}
	report, err := r.collect(ctx, sessionID, time.Time{})
	if err != nil {
		return UsageReport{}, err
	}
	return report.UsageReport, nil
}

func (r *UsageReporter) Summary(ctx context.Context, period UsageSummaryPeriod) (UsageSummary, error) {
	observedAt := r.now().UTC()
	since, err := period.Since(observedAt)
	if err != nil {
		return UsageSummary{}, err
	}
	report, err := r.collect(ctx, "", since)
	if err != nil {
		return UsageSummary{}, err
	}
	report.ObservedAt, report.Since = observedAt, since
	return report, nil
}

func (r *UsageReporter) collect(ctx context.Context, sessionID string, since time.Time) (UsageSummary, error) {
	total := usageAccumulator{}
	groups := []map[string]*usageAccumulator{{}, {}, {}, {}, {}}
	sessions := map[string]struct{}{}
	runs := map[string]struct{}{}
	err := r.source.VisitEndpointInvocations(ctx, sessionID, since, func(source modelref.EndpointObservation) error {
		if err := total.addCall(source); err != nil {
			return fmt.Errorf("sessions: aggregate endpoint %q: %w", source.Invocation.ID, err)
		}
		model := source.ServedModel
		if _, err := modelchoice.New(source.Invocation.Provider, model); err != nil {
			return err
		}
		scope := "unattached"
		if source.SessionID != "" {
			sessions[source.SessionID] = struct{}{}
			scope = "session"
		}
		if source.RunID != "" {
			runs[source.RunID] = struct{}{}
			scope = "run"
		}
		keys := []string{source.Invocation.Provider, source.Invocation.Provider + "/" + model, source.Invocation.StartedAt.UTC().Format(time.DateOnly), string(source.Invocation.Caller.Origin), scope}
		for index, key := range keys {
			if err := accumulatorFor(groups[index], key).addCall(source); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return UsageSummary{}, err
	}
	return UsageSummary{UsageReport: UsageReport{Total: total.usage(), RecordedCostUSD: total.recordedCost(), Coverage: total.coverage, ByModel: bucketsBySpend(groups[1])}, ByProvider: bucketsBySpend(groups[0]), ByDay: bucketsByKey(groups[2]), ByOrigin: bucketsByKey(groups[3]), ByScope: bucketsByKey(groups[4]), Sessions: len(sessions), Runs: len(runs)}, nil
}

type usageAccumulator struct {
	tokens   accounting.Tokens
	cost     accounting.Cost
	coverage UsageCoverage
}

func (u *usageAccumulator) addCall(source modelref.EndpointObservation) error {
	next := *u
	if next.coverage.Calls == math.MaxInt {
		return errors.New("usage call count overflows")
	}
	next.coverage.Calls++
	switch source.State {
	case "started":
		next.coverage.StartedCalls++
	case "completed":
		next.coverage.CompletedCalls++
	case "failed":
		next.coverage.FailedCalls++
	case "unknown":
		next.coverage.UnknownCalls++
	default:
		return errors.New("usage source has an unknown state")
	}
	if (source.State == "completed" || source.State == "failed") != source.HasResponse() {
		return errors.New("usage source outcome differs from its state")
	}
	reported := source.Usage
	if reported != nil {
		tokens, err := accounting.NewTokens(*reported)
		if err != nil {
			return err
		}
		next.tokens, err = next.tokens.Add(tokens)
		if err != nil {
			return err
		}
		next.coverage.ReportedCalls++
	}
	cost, err := accounting.CostFromOptional(source.CostUSD)
	if err != nil {
		return err
	}
	if source.CostUSD != nil {
		if reported == nil {
			return errors.New("usage source price has no reported usage")
		}
		if next.coverage.PricedCalls == 0 {
			next.cost = cost
		} else {
			next.cost, err = next.cost.Add(cost)
			if err != nil {
				return err
			}
		}
		next.coverage.PricedCalls++
	}
	*u = next
	return nil
}

func (u usageAccumulator) recordedCost() *float64 {
	if u.coverage.PricedCalls == 0 {
		return nil
	}
	return u.cost.OptionalUSD()
}

func (u usageAccumulator) usage() accounting.Totals {
	value := accounting.Totals{Tokens: u.tokens}
	if u.coverage.Calls > 0 && u.coverage.PricedCalls == u.coverage.Calls {
		value.CostUSD = u.recordedCost()
	}
	return value
}

func accumulatorFor(groups map[string]*usageAccumulator, key string) *usageAccumulator {
	value := groups[key]
	if value == nil {
		value = &usageAccumulator{}
		groups[key] = value
	}
	return value
}

func bucketsOf(groups map[string]*usageAccumulator) []UsageBucket {
	result := make([]UsageBucket, 0, len(groups))
	for key, value := range groups {
		result = append(result, UsageBucket{Key: key, Usage: value.usage(), RecordedCostUSD: value.recordedCost(), Coverage: value.coverage})
	}
	return result
}

func bucketsBySpend(groups map[string]*usageAccumulator) []UsageBucket {
	result := bucketsOf(groups)
	slices.SortFunc(result, func(a, b UsageBucket) int {
		return cmp.Or(cmp.Compare(costOf(b.RecordedCostUSD), costOf(a.RecordedCostUSD)), cmp.Compare(b.Usage.InputTokens, a.Usage.InputTokens), cmp.Compare(a.Key, b.Key))
	})
	return result
}

func bucketsByKey(groups map[string]*usageAccumulator) []UsageBucket {
	result := bucketsOf(groups)
	slices.SortFunc(result, func(a, b UsageBucket) int { return cmp.Compare(a.Key, b.Key) })
	return result
}

func costOf(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}
