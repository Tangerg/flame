package runs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type acceptedRevivalStore struct {
	value   AcceptedContinuation
	retired bool
}

func (s *acceptedRevivalStore) List(context.Context) ([]AcceptedContinuation, error) {
	return []AcceptedContinuation{s.value.Clone()}, nil
}
func (s *acceptedRevivalStore) Delete(context.Context, string) error { s.retired = true; return nil }

func TestAcceptedRevivalPreservesOwnershipAndInconclusiveFailures(t *testing.T) {
	transient := errors.New("tree store unavailable")
	for _, scenario := range []string{"transient failure", "proven loss", "busy", "segment changed", "terminal"} {
		t.Run(scenario, func(t *testing.T) {
			at := time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC)
			pending := testApprovalPending("member_1", at)
			sessions := &fakeRunSessions{sess: testsupport.MustRestoreSession(session.Snapshot{ID: "ses_1", Workspace: testsupport.MustWorkspace("/work")}), pending: map[string]Pending{"run_1": pending}}
			control := &fakeExecutionPorts{rehydrateErr: transient}
			effects := &fakeEffects{}
			coordinator := newUseCaseCoordinator(&fakeExecutor{}, control, sessions, effects)
			parked := runForPending(pending)
			resumed, err := run.Replace(parked, func(value run.Run) (run.Run, error) { return value.Resume("seg_accepted", at.Add(2*time.Second)) })
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := NewAcceptedContinuation("ses_1", pending, run.TreeResumeDraft{Runs: []run.Replacement{resumed}}, "")
			if err != nil {
				t.Fatal(err)
			}
			store := &acceptedRevivalStore{value: accepted}
			coordinator.acceptedContinuations = store
			coordinator.newSegmentID = func() string { t.Fatal("revival allocated a Segment"); return "" }
			coordinator.runs.(*fakeRunProjection).runs["run_1"] = resumed.State()
			items := coordinator.items.(*fakeItemProjection).items
			acceptance, err := NewResumeAcceptance("ses_1", pending, items, []InterruptAnswer{{InterruptItemID: "item_1", MemberID: "member_1", RequestID: "request_1", Resolution: interrupt.Resolution{Approved: true}}})
			if err != nil {
				t.Fatal(err)
			}
			replacements, err := acceptance.ItemReplacements()
			if err != nil {
				t.Fatal(err)
			}
			for _, replacement := range replacements {
				items[replacement.State().ID()] = replacement.State()
			}
			wantRetired := true
			switch scenario {
			case "transient failure":
				wantRetired = false
			case "proven loss":
				control.rehydrateErr = ErrExecutorStateLost
			case "busy":
				wantRetired = false
				admission, ok, err := coordinator.admission.AcquireRun(t.Context(), "ses_1", "/work")
				if err != nil || !ok {
					t.Fatalf("admission=%t,%v", ok, err)
				}
				defer admission.Release()
			case "segment changed":
				value, err := parked.Resume("seg_other", at.Add(2*time.Second))
				if err != nil {
					t.Fatal(err)
				}
				coordinator.runs.(*fakeRunProjection).runs["run_1"] = value
			case "terminal":
				value, err := resumed.State().Terminate(run.Termination{Outcome: run.OutcomeCanceled, FinishedAt: at.Add(3 * time.Second)})
				if err != nil {
					t.Fatal(err)
				}
				coordinator.runs.(*fakeRunProjection).runs["run_1"] = value
			}
			err = coordinator.ReviveAcceptedContinuations(t.Context())
			if scenario == "transient failure" {
				if !errors.Is(err, transient) {
					t.Fatalf("transient cause lost: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if store.retired != wantRetired {
				t.Fatalf("retired=%t,want %t", store.retired, wantRetired)
			}
			if len(effects.openings) != 0 || control.resumed {
				t.Fatal("refused revival opened or activated execution")
			}
		})
	}
}
