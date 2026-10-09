package sqlite_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestRunReadsRefuseOpenRowsWithTerminalFacts(t *testing.T) {
	for _, state := range []run.State{run.Running, run.Waiting} {
		for _, fact := range []struct {
			name   string
			column string
			value  any
			want   string
		}{
			{"outcome", "outcome", "completed", "carries an outcome"},
			{"failure", "problem", `{"kind":"internal"}`, "carries a failure"},
			{"finish time", "finished_at", runCreatedAt.Add(time.Second).UnixNano(), "carries finish time"},
		} {
			t.Run(string(state)+"/"+fact.name, func(t *testing.T) {
				db, err := sqlite.Open(t.Context(), ":memory:")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Close() })
				store := sqlite.NewRunStore(db)
				draft := runDraft("run_decode", "ses_decode")
				if err := store.Admit(t.Context(), draft); err != nil {
					t.Fatal(err)
				}
				if state == run.Waiting {
					if _, err := db.ExecContext(t.Context(), `UPDATE runs SET state = 'waiting', active_segment_id = ''`); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := db.ExecContext(t.Context(), "UPDATE runs SET "+fact.column+" = ?", fact.value); err != nil {
					t.Fatal(err)
				}
				if _, err := store.ListNonTerminalRuns(t.Context()); err == nil || !strings.Contains(err.Error(), fact.want) {
					t.Fatalf("recovery read error = %v, want Domain rejection of %s", err, fact.name)
				}
				if state == run.Running {
					if _, _, err := store.Run(t.Context(), draft.RunID); err == nil || !strings.Contains(err.Error(), fact.want) {
						t.Fatalf("ordinary read error = %v, want Domain rejection of %s", err, fact.name)
					}
				}
			})
		}
	}
}

func TestRunCodecPreservesEpochFinishTime(t *testing.T) {
	store, _ := newRunStores(t)
	draft := runDraft("run_epoch", "ses_epoch")
	draft.CreatedAt = time.Unix(-1, 0).UTC()
	if err := store.Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	opened := admittedRunFromDraft(draft)
	change := testsupport.MustRunReplacement(opened, func(current run.Run) (run.Run, error) {
		return current.Terminate(run.Termination{
			Outcome: run.OutcomeCompleted, FinishedAt: time.Unix(0, 0).UTC(), MessageMark: run.MessageMarkAt(0),
		})
	})
	if err := store.Terminalize(t.Context(), change); err != nil {
		t.Fatal(err)
	}
	assertStoredRun(t, store, change.State())
}
