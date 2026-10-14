package sessions

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/core/chat"
)

type usageSourceStub struct {
	values  []modelref.EndpointObservation
	failure error
}

func (s usageSourceStub) VisitEndpointInvocations(_ context.Context, sessionID string, since time.Time, yield func(modelref.EndpointObservation) error) error {
	for _, value := range s.values {
		if (sessionID == "" || value.SessionID == sessionID) && (since.IsZero() || !value.Invocation.StartedAt.Before(since)) {
			if err := yield(value); err != nil {
				return err
			}
		}
	}
	return s.failure
}
func TestNewUsageReporterRequiresPhysicalSource(t *testing.T) {
	if _, err := NewUsageReporter(UsageDependencies{}); err == nil {
		t.Fatal("missing source accepted")
	}
	var source *usageSourceStub
	if _, err := NewUsageReporter(UsageDependencies{Source: source}); err == nil {
		t.Fatal("typed nil source accepted")
	}
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

func usd(value float64) *float64 { return &value }
func usageSource(id, provider, model, state string, at time.Time, reported *chat.Usage, cost *float64) modelref.EndpointObservation {
	value := modelref.EndpointObservation{Invocation: modelref.EndpointInvocation{ID: id, Provider: provider, Model: model, StartedAt: at, Caller: capability.Invocation{Origin: capability.Automatic}}, State: state, CostUSD: cost}
	if state == "completed" || state == "failed" {
		message := chat.NewAssistantMessage(chat.NewTextPart("known"))
		value.Response = &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: model, Usage: reported}}
	}
	return value
}
func TestPhysicalUsageIncludesAuxiliaryUnattachedAndUnknownWithoutDoubleCounting(t *testing.T) {
	at := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	primary := usageSource("primary", "provider-a", "selected", "completed", at, &chat.Usage{InputTokens: 100, OutputTokens: 40, CacheReadInputTokens: new(int64(3)), ReasoningTokens: new(int64(2))}, usd(1.5))
	primary.SessionID = "ses_one"
	primary.RunID = "run_one"
	primary.Response.Metadata.Model = "served"
	auxiliary := usageSource("title", "provider-b", "served", "failed", at.Add(time.Second), &chat.Usage{InputTokens: 10, OutputTokens: 2}, usd(0))
	auxiliary.SessionID = "ses_one"
	unknown := usageSource("global", "provider-a", "served", "unknown", at.Add(2*time.Second), nil, nil)
	unreported := usageSource("unreported", "provider-a", "served", "completed", at.Add(3*time.Second), nil, nil)
	started := usageSource("started", "provider-b", "served", "started", at.Add(4*time.Second), nil, nil)
	reporter, err := NewUsageReporter(UsageDependencies{Source: usageSourceStub{values: []modelref.EndpointObservation{primary, auxiliary, unknown, unreported, started}}})
	if err != nil {
		t.Fatal(err)
	}
	report, err := reporter.Summary(t.Context(), AllTimeUsage())
	if err != nil {
		t.Fatal(err)
	}
	want := UsageCoverage{Calls: 5, ReportedCalls: 2, PricedCalls: 2, CompletedCalls: 2, FailedCalls: 1, UnknownCalls: 1, StartedCalls: 1}
	if report.Coverage != want || report.Total.InputTokens != 110 || report.Total.OutputTokens != 42 || report.Total.CacheReadTokens != 3 || report.Total.ReasoningTokens != 2 || report.Total.CostUSD != nil || report.RecordedCostUSD == nil || *report.RecordedCostUSD != 1.5 || report.Runs != 1 || report.Sessions != 1 {
		t.Fatalf("physical total changed or hid incomplete coverage: %+v", report)
	}
	if len(report.ByModel) != 2 || report.ByModel[0].Key != "provider-a/served" || report.ByModel[1].Key != "provider-b/served" {
		t.Fatalf("served provider/model identity lost: %+v", report.ByModel)
	}
	for _, groups := range [][]UsageBucket{report.ByProvider, report.ByModel, report.ByDay, report.ByOrigin, report.ByScope} {
		calls := 0
		var input int64
		for _, group := range groups {
			calls += group.Coverage.Calls
			input += group.Usage.InputTokens
		}
		if calls != report.Coverage.Calls || input != report.Total.InputTokens {
			t.Fatalf("grouping does not reconcile: %+v", groups)
		}
	}
	session, err := reporter.Session(t.Context(), "ses_one")
	if err != nil {
		t.Fatal(err)
	}
	if session.Coverage.Calls != 2 || session.Total.CostUSD == nil || *session.Total.CostUSD != 1.5 || session.Total.InputTokens != 110 {
		t.Fatalf("Session-only auxiliary missing: %+v", session)
	}
}
func TestPhysicalUsageFiltersAtAdmissionAndRejectsPartialRead(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	current := usageSource("current", "provider", "model", "completed", now, &chat.Usage{}, usd(0))
	old := usageSource("old", "provider", "model", "completed", now.AddDate(0, 0, -8), &chat.Usage{InputTokens: 1}, usd(2))
	reader, err := NewUsageReporter(UsageDependencies{Source: usageSourceStub{values: []modelref.EndpointObservation{old, current}}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	period, _ := RecentUsageDays(7)
	report, err := reader.Summary(t.Context(), period)
	if err != nil || report.Coverage.Calls != 1 || report.Coverage.ReportedCalls != 1 || report.Total.CostUSD == nil || *report.Total.CostUSD != 0 {
		t.Fatalf("window or reported zero lost: %+v %v", report, err)
	}
	reader, err = NewUsageReporter(UsageDependencies{Source: usageSourceStub{values: []modelref.EndpointObservation{current}, failure: errors.New("later source is corrupt")}})
	if err != nil {
		t.Fatal(err)
	}
	if report, err := reader.Summary(t.Context(), AllTimeUsage()); err == nil || report.Coverage.Calls != 0 {
		t.Fatal("partial usage returned as complete")
	}
}
func TestAccumulatorRejectsOverflowWithoutMutation(t *testing.T) {
	value := usageAccumulator{}
	first := usageSource("first", "provider", "model", "completed", time.Now().UTC(), &chat.Usage{InputTokens: math.MaxInt64}, usd(1))
	if err := value.addCall(first); err != nil {
		t.Fatal(err)
	}
	before := value
	if err := value.addCall(usageSource("next", "provider", "model", "completed", time.Now().UTC(), &chat.Usage{InputTokens: 1}, usd(1))); err == nil || !reflect.DeepEqual(before, value) {
		t.Fatal("overflow mutated aggregate")
	}
}
func TestBucketsBySpendRankCostThenUsageThenIdentity(t *testing.T) {
	values := map[string]*usageAccumulator{}
	for index, name := range []string{"z", "a", "highest"} {
		value := usageSource(fmt.Sprint(index), "provider", name, "completed", time.Now().UTC(), &chat.Usage{InputTokens: 1}, usd(1))
		if name == "highest" {
			value.CostUSD = usd(2)
		}
		if err := accumulatorFor(values, name).addCall(value); err != nil {
			t.Fatal(err)
		}
	}
	groups := bucketsBySpend(values)
	if groups[0].Key != "highest" || groups[1].Key != "a" || groups[2].Key != "z" {
		t.Fatalf("unstable spend ordering: %+v", groups)
	}
}
