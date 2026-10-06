package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestModeStoreTracksPlanEntryExitAndSessionLifecycle(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	modes, sessions := sqlite.NewModeStore(db), sqlite.NewSessionStore(db)
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

// The default a user chose is Runtime state, so it outlives the process that
// recorded it instead of reverting to the product default on restart.
func TestModeStoreKeepsTheChosenDefaultAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	modes := sqlite.NewModeStore(db)
	if mode, found, err := modes.DefaultMode(t.Context()); err != nil || found {
		t.Fatalf("unchosen DefaultMode = %q, %v, %v; want not found", mode, found, err)
	}
	for _, mode := range []approval.Mode{approval.ModeYolo, approval.ModeSafe} {
		if err := modes.SetDefaultMode(t.Context(), mode); err != nil {
			t.Fatalf("SetDefaultMode(%s): %v", mode, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if mode, found, err := sqlite.NewModeStore(reopened).DefaultMode(t.Context()); err != nil || !found || mode != approval.ModeSafe {
		t.Fatalf("reopened DefaultMode = %q, %v, %v; want safe", mode, found, err)
	}
	if err := sqlite.NewModeStore(reopened).SetDefaultMode(t.Context(), approval.ModePlan); err == nil {
		t.Fatal("SetDefaultMode(plan) succeeded; Plan is session-only")
	}
}
