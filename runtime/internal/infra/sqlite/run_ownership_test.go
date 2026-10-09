package sqlite_test

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestRunWritesCannotRegressCommittedProgress(t *testing.T) {
	for _, operation := range []string{"progress", "suspend", "terminal event", "terminal replacement", "lost replacement"} {
		t.Run(operation, func(t *testing.T) {
			store, _ := newRunStores(t)
			draft := runDraft("run_progress", "ses_progress")
			if err := store.Admit(t.Context(), draft); err != nil {
				t.Fatal(err)
			}
			at := runCreatedAt.Add(time.Second)
			stale := testsupport.MustProgressedRun(draft, testsupport.MustRunMetrics(testsupport.RunMetricsInput{Steps: 1}), 10, at)
			current := testsupport.MustProgressedRun(draft, testsupport.MustRunMetrics(testsupport.RunMetricsInput{Steps: 2}), 20, at)
			if err := store.UpdateProgress(t.Context(), current); err != nil {
				t.Fatal(err)
			}
			var err error
			switch operation {
			case "progress":
				err = store.UpdateProgress(t.Context(), stale)
			case "suspend":
				parked, parkErr := stale.Suspend(at)
				if parkErr != nil {
					t.Fatal(parkErr)
				}
				err = store.Suspend(t.Context(), parked, draft.SegmentID, runtimeidentity.CommitID{})
			case "terminal event", "terminal replacement":
				terminal, terminalErr := stale.Terminate(run.Termination{Outcome: run.OutcomeCompleted, FinishedAt: at, MessageMark: run.MessageMarkAt(0)})
				if terminalErr != nil {
					t.Fatal(terminalErr)
				}
				if operation == "terminal event" {
					err = store.TerminalizeEvent(t.Context(), terminal, draft.SegmentID, runtimeidentity.NewCommit())
				} else {
					err = store.Terminalize(t.Context(), testsupport.MustRunReplacement(stale, testsupport.DecidedRun(terminal)))
				}
			case "lost replacement":
				lost, lostErr := stale.RecoverLost(run.Failure{Kind: run.FailureLost}, at, run.MessageMarkAt(0))
				if lostErr != nil {
					t.Fatal(lostErr)
				}
				err = store.RecoverLost(t.Context(), testsupport.MustRunReplacement(stale, testsupport.DecidedRun(lost)))
			}
			if err == nil {
				t.Fatal("stale Run snapshot overwrote committed progress at the same timestamp")
			}
			assertStoredRun(t, store, current)
		})
	}
}

func TestResumeRequiresCompleteExpectedRun(t *testing.T) {
	store, _ := newRunStores(t)
	draft := runDraft("run_resume", "ses_resume")
	if err := store.Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	parked := parkedRunFromDraft(draft)
	if err := suspendRun(t.Context(), store, parked, draft.SegmentID); err != nil {
		t.Fatal(err)
	}
	snapshot := parked.Snapshot()
	snapshot.ContextTokens = 100
	foreign := testsupport.MustRestoreRun(snapshot)
	if err := resumeParkedRun(t.Context(), store, foreign, "seg_resumed", parked.UpdatedAt()); err == nil {
		t.Fatal("resume accepted another source snapshot with the same lifecycle coordinates")
	}
	assertStoredOpenRun(t, store, parked)
	if err := resumeParkedRun(t.Context(), store, parked, "seg_resumed", parked.UpdatedAt()); err != nil {
		t.Fatalf("exact resume: %v", err)
	}
}

func TestResumePersistsCompleteDecidedProgress(t *testing.T) {
	store, _ := newRunStores(t)
	draft := runDraft("run_resume", "ses_resume")
	if err := store.Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	parked := parkedRunFromDraft(draft)
	if err := suspendRun(t.Context(), store, parked, draft.SegmentID); err != nil {
		t.Fatal(err)
	}
	metrics := testsupport.MustRunMetrics(testsupport.RunMetricsInput{Steps: 2, ActiveDuration: time.Second})
	change := testsupport.MustRunReplacement(parked, func(current run.Run) (run.Run, error) {
		return current.AdvanceProgress(metrics, 100, current.UpdatedAt())
	})
	change, err := change.Then(func(current run.Run) (run.Run, error) {
		return current.Resume("seg_resumed", current.UpdatedAt())
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Resume(t.Context(), change); err != nil {
		t.Fatal(err)
	}
	assertStoredRun(t, store, change.State())
}

func TestMessageMarkRebaseRequiresCompleteExpectedRun(t *testing.T) {
	store, _ := newRunStores(t)
	terminal := finishedRun("run_terminal", "ses_terminal", run.OutcomeCompleted)
	if err := store.Restore(t.Context(), terminal); err != nil {
		t.Fatal(err)
	}
	snapshot := terminal.Snapshot()
	snapshot.ContextTokens = 100
	foreign := testsupport.MustRestoreRun(snapshot)
	change := testsupport.MustRunReplacement(foreign, func(current run.Run) (run.Run, error) {
		return current.WithMessageMark(2)
	})
	if err := store.RebaseMessageMark(t.Context(), change); err == nil {
		t.Fatal("message watermark rebase accepted another terminal source snapshot")
	}
	assertStoredRun(t, store, terminal)
	change = testsupport.MustRunReplacement(terminal, func(current run.Run) (run.Run, error) {
		return current.WithMessageMark(2)
	})
	if err := store.RebaseMessageMark(t.Context(), change); err != nil {
		t.Fatalf("exact message watermark rebase: %v", err)
	}
	assertStoredRun(t, store, change.State())
}

func assertStoredRun(t *testing.T, store *sqlite.RunStore, want run.Run) {
	t.Helper()
	got, found, err := store.Run(t.Context(), want.ID())
	if err != nil || !found || !got.Equal(want) {
		t.Fatalf("stored Run differs from its domain owner: found=%t got=%+v want=%+v err=%v", found, got.Snapshot(), want.Snapshot(), err)
	}
}

func assertStoredOpenRun(t *testing.T, store *sqlite.RunStore, want run.Run) {
	t.Helper()
	got, err := store.ListNonTerminalRuns(t.Context())
	if err != nil || len(got) != 1 || !got[0].Equal(want) {
		t.Fatalf("stored open Runs differ from their domain owner: got=%+v want=%+v err=%v", got, want.Snapshot(), err)
	}
}
