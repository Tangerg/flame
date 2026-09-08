package agent

import (
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestConversationOwnsMutableRunFacts(t *testing.T) {
	run := protocol.RunRef{
		RunSummary: protocol.RunSummary{
			ID: "run_1", SessionID: "ses_1", Provider: "mock", Model: "balanced",
			Status: protocol.RunStatusFinished, CreatedAt: time.Unix(1, 0).UTC(), FinishedAt: time.Unix(2, 0).UTC(),
			Outcome: &protocol.RunOutcome{Type: protocol.OutcomeFailed, Error: &protocol.ProblemData{Type: protocol.ProblemRateLimited, Detail: "quota exhausted"}},
		},
		Limits: &protocol.RunLimits{MaxTotalTokens: new(int64(100)), MaxSteps: new(10), MaxBudgetUSD: new(2.0)},
		Metrics: protocol.RunMetrics{Usage: &protocol.Usage{
			ModelUsage: protocol.ModelUsage{InputTokens: 20, CostUSD: new(0.1)},
			ByModel:    map[string]protocol.ModelUsage{"mock/balanced": {InputTokens: 20, CostUSD: new(0.1)}},
		}},
		ProtocolProfile: protocol.RunProtocolProfile{RequiredFeatures: []protocol.RunProtocolFeature{protocol.RunProtocolFeatureSubagents}, InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval}},
	}
	conversation := NewConversation()
	conversation.RestoreSnapshot(SessionSnapshot{Runs: []protocol.RunRef{run}})
	mutate := func(value protocol.RunRef) {
		value.Outcome.Error.Detail = "changed"
		*value.Limits.MaxTotalTokens, *value.Limits.MaxSteps, *value.Limits.MaxBudgetUSD = 1, 1, 1
		value.Metrics.Usage.InputTokens = 1
		*value.Metrics.Usage.CostUSD = 1
		model := value.Metrics.Usage.ByModel["mock/balanced"]
		*model.CostUSD = 1
		delete(value.Metrics.Usage.ByModel, "mock/balanced")
		value.ProtocolProfile.RequiredFeatures[0] = "changed"
		value.ProtocolProfile.InterruptTypes[0] = protocol.InterruptQuestion
	}
	requireRetained := func() protocol.RunRef {
		t.Helper()
		got, exists := conversation.CurrentRun()
		if !exists || got.Outcome.Error.Detail != "quota exhausted" || *got.Limits.MaxTotalTokens != 100 || *got.Limits.MaxSteps != 10 || *got.Limits.MaxBudgetUSD != 2 || got.Metrics.Usage.InputTokens != 20 || *got.Metrics.Usage.CostUSD != 0.1 || len(got.Metrics.Usage.ByModel) != 1 || *got.Metrics.Usage.ByModel["mock/balanced"].CostUSD != 0.1 || got.ProtocolProfile.RequiredFeatures[0] != protocol.RunProtocolFeatureSubagents || got.ProtocolProfile.InterruptTypes[0] != protocol.InterruptApproval {
			t.Fatalf("retained Run changed: %+v", got)
		}
		return got
	}
	mutate(run)
	mutate(requireRetained())
	requireRetained()
	mutate(conversation.Runs()[0])
	requireRetained()
}

func TestConversationPreservesReportedZeroMeteringAcrossRecovery(t *testing.T) {
	for _, test := range []struct {
		name  string
		usage *protocol.Usage
	}{
		{name: "unreported"},
		{name: "reported zero", usage: &protocol.Usage{}},
		{name: "priced zero", usage: &protocol.Usage{ModelUsage: protocol.ModelUsage{CostUSD: new(0.0)}, ByModel: map[string]protocol.ModelUsage{"mock/balanced": {CostUSD: new(0.0)}}}},
	} {
		name, reported := test.name, test.usage != nil
		t.Run(name, func(t *testing.T) {
			conversation := NewConversation()
			run := runningRun("seg_1")
			apply(t, conversation, RunEvent{EventID: "start", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: SegmentStarted{Run: run}})
			metrics := protocol.RunMetrics{Steps: 1, ActiveDurationMillis: 17, Usage: test.usage}
			apply(t, conversation, RunEvent{EventID: "progress", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: RunProgress{Usage: metrics.Usage}})
			progress, _ := conversation.CurrentRun()
			if (progress.Metrics.Usage != nil) != reported {
				t.Fatalf("progress lost metering presence: %+v", progress.Metrics)
			}
			apply(t, conversation, RunEvent{EventID: "finish", RunID: run.ID, SegmentID: run.ActiveSegmentID, Event: RunFinished{Outcome: Outcome{Status: protocol.OutcomeCompleted}, Metrics: metrics}})
			run.Status, run.ActiveSegmentID = protocol.RunStatusFinished, ""
			run.Outcome = &protocol.RunOutcome{Type: protocol.OutcomeCompleted}
			run.Metrics = CloneRunMetrics(metrics)
			snapshot := SessionSnapshot{Runs: []protocol.RunRef{run}}
			if current, _ := conversation.CurrentRun(); !reflect.DeepEqual(current, run) {
				t.Fatal("live metering differs from the authoritative cold snapshot")
			}
			if metrics.Usage != nil && metrics.Usage.CostUSD != nil {
				*metrics.Usage.CostUSD = 9
				delete(metrics.Usage.ByModel, "mock/balanced")
				if current, _ := conversation.CurrentRun(); !reflect.DeepEqual(current, run) {
					t.Fatal("retained metering changed with its source event")
				}
			}
			conversation.RestoreSnapshot(snapshot)
			recovered, _ := conversation.CurrentRun()
			if !reflect.DeepEqual(recovered, run) {
				t.Fatalf("recovered metering = %+v", recovered.Metrics)
			}
		})
	}
}
