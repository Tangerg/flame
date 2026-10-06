package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestPlanModeStoreTracksEntryExitAndSessionLifecycle(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	modes, sessions := sqlite.NewPlanModeStore(db), sqlite.NewSessionStore(db)
	created := testsupport.MustRestoreSession(session.Snapshot{
		ID: "ses_plan", Title: "Plan session", Workspace: testsupport.MustWorkspace("/repo"),
	})
	if err := sessions.Insert(t.Context(), created); err != nil {
		t.Fatalf("create session: %v", err)
	}
	active := func(want bool) {
		t.Helper()
		if got, err := modes.PlanModeActive(t.Context(), created.ID()); err != nil || got != want {
			t.Fatalf("PlanModeActive = %v, %v; want %v", got, err, want)
		}
	}

	active(false)
	if changed, err := modes.StartPlanMode(t.Context(), created.ID()); err != nil || !changed {
		t.Fatalf("StartPlanMode = %v, %v", changed, err)
	}
	if changed, err := modes.StartPlanMode(t.Context(), created.ID()); err != nil || changed {
		t.Fatalf("repeated StartPlanMode = %v, %v; want unchanged", changed, err)
	}
	active(true)
	if changed, err := modes.EndPlanMode(t.Context(), created.ID()); err != nil || !changed {
		t.Fatalf("EndPlanMode = %v, %v", changed, err)
	}
	active(false)
	if changed, err := modes.EndPlanMode(t.Context(), created.ID()); err != nil || changed {
		t.Fatalf("repeated EndPlanMode = %v, %v; want unchanged", changed, err)
	}

	if _, err := modes.StartPlanMode(t.Context(), created.ID()); err != nil {
		t.Fatal(err)
	}
	if err := sessions.Delete(t.Context(), created.ID()); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	active(false)
}
