package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestTerminalEffectsSurviveDatabaseReopen(t *testing.T) {
	for _, outcome := range []run.Outcome{run.OutcomeCanceled, run.OutcomeTimedOut, run.OutcomeLost} {
		t.Run(string(outcome), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "effects.db")
			db, err := sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			store := sqlite.NewRunStore(db)
			draft := runDraft("run_effect", "session_effect")
			if err := store.Admit(t.Context(), draft); err != nil {
				t.Fatal(err)
			}
			value := admittedRunFromDraft(draft)
			evidence, err := run.NewUnresolvedEffect("process-tool", "effect-tool", "host_cancellation", "operator stopped", "result unknown")
			if err != nil {
				t.Fatal(err)
			}
			var failure *run.Failure
			if outcome == run.OutcomeTimedOut {
				failure = &run.Failure{Kind: run.FailureTimeout}
			}
			if outcome == run.OutcomeLost {
				failure = &run.Failure{Kind: run.FailureLost}
			}
			terminal, err := value.Terminate(run.Termination{Outcome: outcome, Failure: failure, FinishedAt: time.Unix(9, 0), MessageMark: 0, UnresolvedEffects: []run.UnresolvedEffect{evidence}})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Terminalize(t.Context(), storedRunReplacement(t, t.Context(), store, terminal)); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			restored, found, err := sqlite.NewRunStore(db).Run(t.Context(), terminal.ID())
			if err != nil || !found || !restored.Equal(terminal) {
				t.Fatalf("reopened terminal lost evidence: %+v, %v", restored, err)
			}
			effects := restored.UnresolvedEffects()
			effects[0] = run.UnresolvedEffect{}
			if restored.UnresolvedEffects()[0] != evidence {
				t.Fatal("caller mutated evidence")
			}
		})
	}
}
