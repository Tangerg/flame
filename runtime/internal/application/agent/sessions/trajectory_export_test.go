package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
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
