package persistence

import (
	corecontent "github.com/Tangerg/scope/core/content"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/testsupport"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func TestTerminalEffectsSurviveDatabaseReopen(t *testing.T) {
	for _, outcome := range []run.Outcome{run.OutcomeCanceled, run.OutcomeTimedOut, run.OutcomeLost} {
		t.Run(string(outcome), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "effects.db")
			db, err := openDatabase(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			store := newRunStore(databaseFor(db))
			draft := runDraft("run_effect", "session_effect")
			if err := store.Admit(t.Context(), draft); err != nil {
				t.Fatal(err)
			}
			value := admittedRunFromDraft(draft)
			output := toolcontract.Output{Content: []corecontent.Block{{Kind: corecontent.Text, Text: "observed before cancellation"}}, Details: []byte(`{"receipt":9007199254740993}`)}
			evidence, err := run.NewUnresolvedEffect(run.UnresolvedEffectConfig{ProcessID: "process-tool", EffectID: "effect-tool", Cause: "host_cancellation", Reason: "operator stopped", Detail: "result unknown", Output: &output})
			if err != nil {
				t.Fatal(err)
			}
			terminal, err := value.Terminate(run.Termination{Ending: testsupport.EndingFor(outcome), FinishedAt: time.Unix(9, 0), MessageMark: run.MessageMarkAt(0), UnresolvedEffects: []run.UnresolvedEffect{evidence}})
			if err != nil {
				t.Fatal(err)
			}
			if err := terminalizeStoredRun(t, t.Context(), store, terminal); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = openDatabase(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			restored, found, err := newRunStore(databaseFor(db)).Run(t.Context(), terminal.ID())
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
