package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type trajectoryEvidenceReader struct {
	evidence TrajectoryEvidence
	err      error
	reads    int
}

func (r *trajectoryEvidenceReader) ReadTrajectoryExport(context.Context, string) (TrajectoryEvidence, error) {
	r.reads++
	return r.evidence, r.err
}

func TestTrajectoryExportReleasesAdmissionAndReturnsNoPartialEvidence(t *testing.T) {
	broken := errors.New("snapshot interrupted")
	for _, test := range []struct {
		name   string
		active bool
		parked bool
		fail   error
	}{
		{name: "active", active: true}, {name: "parked", parked: true},
		{name: "snapshot failure", fail: broken},
	} {
		t.Run(test.name, func(t *testing.T) {
			stores := coordinatorStores{interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{}}}
			if test.parked {
				stores.interrupts.pending["run_1"] = testPending("run_1", time.Unix(1, 0).UTC())
			}
			claimer := &testClaimer{claimed: map[string]bool{"ses_1": test.active}}
			reader := &trajectoryEvidenceReader{evidence: TrajectoryEvidence{Snapshot: portableSnapshot()}, err: test.fail}
			exporter, err := NewTrajectoryExporter(newCoordinatorWithAdmissions(stores, nil, claimer), reader)
			if err != nil {
				t.Fatal(err)
			}
			value, err := exporter.Export(t.Context(), "ses_1")
			if value.Evidence.Snapshot.Session.ID() != "" || err == nil {
				t.Fatalf("failed export returned partial evidence: %+v, %v", value, err)
			}
			if test.active || test.parked {
				if !errors.Is(err, ErrSessionBusy) || reader.reads != 0 {
					t.Fatalf("busy export read history: reads=%d, err=%v", reader.reads, err)
				}
			} else if !errors.Is(err, broken) {
				t.Fatalf("source failure = %v", err)
			}
			if !test.active && claimer.claimed["ses_1"] {
				t.Fatal("failed export leaked its Session admission")
			}
		})
	}
}

func TestTrajectoryToolAttemptsValidateHistoryWithoutReinterpretingSettlement(t *testing.T) {
	at := time.Unix(100, 0).UTC()
	evidence := TrajectoryEvidence{Snapshot: portableSnapshot()}
	evidence.Snapshot.Items = []transcript.Item{testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID: "ses_1", ID: "item_tool", RunID: "run_1", Kind: transcript.ToolCall,
		Status: transcript.ItemIncomplete, OccurredAt: at,
	})}
	evidence.ToolAttempts = []RecordedToolAttempt{
		{RunID: "run_1", Invocation: runs.ToolInvocationCommit{CallID: "call_tool", ItemID: "item_tool", SegmentID: "seg_before", State: runs.ToolInvocationIncomplete, StartedAt: at, FinishedAt: at.Add(time.Second)}},
		{RunID: "run_1", Invocation: runs.ToolInvocationCommit{CallID: "call_tool", ItemID: "item_tool", SegmentID: "seg_after", State: runs.ToolInvocationStarted, StartedAt: at.Add(time.Second)}},
	}
	if err := evidence.Validate(); err != nil {
		t.Fatalf("separate incomplete and unconfirmed attempts = %v", err)
	}
	for _, test := range []struct {
		name string
		edit func(*TrajectoryEvidence)
	}{
		{"missing item", func(e *TrajectoryEvidence) { e.Snapshot.Items = nil }},
		{"non-tool item", func(e *TrajectoryEvidence) {
			e.Snapshot.Items = portableSnapshot().Items
			e.ToolAttempts[0].Invocation.ItemID = "item_1"
		}},
		{"wrong Run", func(e *TrajectoryEvidence) {
			e.Snapshot.Runs = append(e.Snapshot.Runs, testsupport.MustRestoreRun(run.Snapshot{ID: "run_other", SessionID: "ses_1", State: run.Completed}))
			e.ToolAttempts[0].RunID = "run_other"
		}},
		{"duplicate segment", func(e *TrajectoryEvidence) { e.ToolAttempts[1].Invocation.SegmentID = "seg_before" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := evidence
			copy.ToolAttempts = append([]RecordedToolAttempt(nil), evidence.ToolAttempts...)
			test.edit(&copy)
			if err := copy.Validate(); err == nil {
				t.Fatal("invalid Tool evidence accepted")
			}
		})
	}
}
