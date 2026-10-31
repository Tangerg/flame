package sessions

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/qimu/portable"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/qimu/jsonvalue"
	"github.com/Tangerg/scope/core/chat"
)

type usageSourceStub struct {
	values  []modelref.EndpointRecord
	failure error
}

func (s usageSourceStub) VisitEndpointInvocations(_ context.Context, sessionID string, since time.Time, yield func(modelref.EndpointObservation) error) error {
	for _, value := range s.values {
		if (sessionID == "" || value.SessionID == sessionID) && (since.IsZero() || !value.Invocation.StartedAt.Before(since)) {
			observation, err := modelref.RestoreEndpointObservation(value)
			if err != nil {
				return err
			}
			if err := yield(observation); err != nil {
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
	if _, err := NewUsageReporter(UsageDependencies{Source: source, Now: time.Now}); err == nil {
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
		if _, err := RecentUsageDays(days); !errors.Is(err, errInvalidUsageSummaryPeriod) {
			t.Fatalf("RecentDays(%d) = %v", days, err)
		}
	}
	if _, err := (UsageSummaryPeriod{days: 1}).Since(now); !errors.Is(err, errInvalidUsageSummaryPeriod) {
		t.Fatalf("corrupt all-time period = %v", err)
	}
}

func TestSummaryPublishesItsSingleCoreClockObservationAndExactRange(t *testing.T) {
	now := time.Date(2026, 10, 20, 1, 0, 0, 0, time.FixedZone("test", 8*60*60))
	observations := 0
	reporter, err := NewUsageReporter(UsageDependencies{Source: usageSourceStub{}, Now: func() time.Time { observations++; return now }})
	if err != nil {
		t.Fatal(err)
	}
	period, err := RecentUsageDays(7)
	if err != nil {
		t.Fatal(err)
	}
	report, err := reporter.Summary(t.Context(), period)
	if err != nil || observations != 1 || report.ObservedAt != now.UTC() || report.Since != now.UTC().AddDate(0, 0, -7) {
		t.Fatalf("published accounting range: %+v observations=%d error=%v", report, observations, err)
	}
}

func usd(value float64) *float64 { return &value }
func usageRecord(t *testing.T, id, provider, model string, state modelref.InvocationState, at time.Time, reported *chat.Usage, cost *float64) modelref.EndpointRecord {
	t.Helper()
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("input")))
	if err != nil {
		t.Fatal(err)
	}
	caller := "call_" + id
	invocation := modelref.EndpointInvocation{ID: id, Provider: provider, Model: model, StartedAt: at, Caller: portable.Invocation{ID: caller, Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeCall, Reference: caller}}, Request: modelref.ChatEndpointRequest{Chat: request}}
	input, err := jsonvalue.Encode(invocation)
	if err != nil {
		t.Fatal(err)
	}
	requestJSON, err := jsonvalue.Encode(request)
	if err != nil {
		t.Fatal(err)
	}
	value := modelref.EndpointRecord{Invocation: invocation, InvocationJSON: input, RequestJSON: requestJSON, State: state, Cost: testsupport.OptionalCost(cost), Usage: reported}
	if state != modelref.InvocationStarted {
		value.FinishedAt = at.Add(time.Second)
	}
	if state == modelref.InvocationFailed || state == modelref.InvocationUnknown {
		value.Failure = "observed failure"
	}
	if state.HasResponse() {
		value.ResponseJSON = usageResponse(t, model, reported)
	}
	return value
}

func usageResponse(t *testing.T, model string, reported *chat.Usage) jsonvalue.Value {
	t.Helper()
	message := chat.NewAssistantMessage(chat.NewTextPart("known"))
	value, err := jsonvalue.Encode(&chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: model, Usage: reported}})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func usageObservation(t *testing.T, record modelref.EndpointRecord) modelref.EndpointObservation {
	t.Helper()
	value, err := modelref.RestoreEndpointObservation(record)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestPhysicalUsageIncludesAuxiliaryUnattachedAndUnknownWithoutDoubleCounting(t *testing.T) {
	at := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	primary := usageRecord(t, "primary", "provider-a", "selected", modelref.InvocationCompleted, at, &chat.Usage{InputTokens: 100, OutputTokens: 40, CacheReadInputTokens: new(int64(3)), ReasoningTokens: new(int64(2))}, usd(1.5))
	primary.SessionID = "ses_one"
	primary.RunID = "run_one"
	primary.SegmentID = "seg_one"
	primary.ResponseJSON = usageResponse(t, "served", primary.Usage)
	auxiliary := usageRecord(t, "title", "provider-b", "served", modelref.InvocationFailed, at.Add(time.Second), &chat.Usage{InputTokens: 10, OutputTokens: 2}, usd(0))
	auxiliary.SessionID = "ses_one"
	unknown := usageRecord(t, "global", "provider-a", "served", modelref.InvocationUnknown, at.Add(2*time.Second), nil, nil)
	unreported := usageRecord(t, "unreported", "provider-a", "served", modelref.InvocationCompleted, at.Add(3*time.Second), nil, nil)
	started := usageRecord(t, "started", "provider-b", "served", modelref.InvocationStarted, at.Add(4*time.Second), nil, nil)
	reporter, err := NewUsageReporter(UsageDependencies{Source: usageSourceStub{values: []modelref.EndpointRecord{primary, auxiliary, unknown, unreported, started}}, Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	report, err := reporter.Summary(t.Context(), AllTimeUsage())
	if err != nil {
		t.Fatal(err)
	}
	want := UsageCoverage{Calls: 5, ReportedCalls: 2, PricedCalls: 2, CompletedCalls: 2, FailedCalls: 1, UnknownCalls: 1, StartedCalls: 1}
	if report.Coverage != want || report.Total.InputTokens != 110 || report.Total.OutputTokens != 42 || report.Total.CacheReadTokens != 3 || report.Total.ReasoningTokens != 2 || report.Total.Cost.OptionalUSD() != nil || report.RecordedCost.OptionalUSD() == nil || *report.RecordedCost.OptionalUSD() != 1.5 || report.Runs != 1 || report.Sessions != 1 {
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
	if session.Coverage.Calls != 2 || session.Total.Cost.OptionalUSD() == nil || *session.Total.Cost.OptionalUSD() != 1.5 || session.Total.InputTokens != 110 {
		t.Fatalf("Session-only auxiliary missing: %+v", session)
	}
}
func TestPhysicalUsageFiltersAtAdmissionAndRejectsPartialRead(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	current := usageRecord(t, "current", "provider", "model", modelref.InvocationCompleted, now, &chat.Usage{}, usd(0))
	old := usageRecord(t, "old", "provider", "model", modelref.InvocationCompleted, now.AddDate(0, 0, -8), &chat.Usage{InputTokens: 1}, usd(2))
	reader, err := NewUsageReporter(UsageDependencies{Source: usageSourceStub{values: []modelref.EndpointRecord{old, current}}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	period, _ := RecentUsageDays(7)
	report, err := reader.Summary(t.Context(), period)
	if err != nil || report.Coverage.Calls != 1 || report.Coverage.ReportedCalls != 1 || report.Total.Cost.OptionalUSD() == nil || *report.Total.Cost.OptionalUSD() != 0 {
		t.Fatalf("window or reported zero lost: %+v %v", report, err)
	}
	reader, err = NewUsageReporter(UsageDependencies{Source: usageSourceStub{values: []modelref.EndpointRecord{current}, failure: errors.New("later source is corrupt")}, Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	if report, err := reader.Summary(t.Context(), AllTimeUsage()); err == nil || report.Coverage.Calls != 0 {
		t.Fatal("partial usage returned as complete")
	}
}
func TestAccumulatorRejectsOverflowWithoutMutation(t *testing.T) {
	value := usageAccumulator{}
	first := usageRecord(t, "first", "provider", "model", modelref.InvocationCompleted, time.Now().UTC(), &chat.Usage{InputTokens: math.MaxInt64}, usd(1))
	if err := value.addCall(usageObservation(t, first)); err != nil {
		t.Fatal(err)
	}
	before := value
	if err := value.addCall(usageObservation(t, usageRecord(t, "next", "provider", "model", modelref.InvocationCompleted, time.Now().UTC(), &chat.Usage{InputTokens: 1}, usd(1)))); err == nil || !reflect.DeepEqual(before, value) {
		t.Fatal("overflow mutated aggregate")
	}
}
func TestBucketsBySpendRankCostThenUsageThenIdentity(t *testing.T) {
	values := map[string]*usageAccumulator{}
	for index, name := range []string{"z", "a", "highest"} {
		value := usageRecord(t, fmt.Sprint(index), "provider", name, modelref.InvocationCompleted, time.Now().UTC(), &chat.Usage{InputTokens: 1}, usd(1))
		if name == "highest" {
			value.Cost = testsupport.OptionalCost(usd(2))
		}
		if err := accumulatorFor(values, name).addCall(usageObservation(t, value)); err != nil {
			t.Fatal(err)
		}
	}
	groups := bucketsBySpend(values)
	if groups[0].Key != "highest" || groups[1].Key != "a" || groups[2].Key != "z" {
		t.Fatalf("unstable spend ordering: %+v", groups)
	}
}
