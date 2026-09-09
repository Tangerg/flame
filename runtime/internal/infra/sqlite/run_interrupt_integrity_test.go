package sqlite_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

// TestWaitingRunReadRejectsACorruptPendingSet covers the reason the Run read
// joins the interrupt payload at all. A Run carries no interrupts of its own, so
// the join earns its cost only by proving the parked set is intact: without it a
// corrupt payload would read back as a healthy waiting Run and fail later,
// somewhere that cannot name which Run is unusable.
func TestWaitingRunReadRejectsACorruptPendingSet(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	runStore, interrupts := sqlite.NewRunStore(db), persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	ctx := context.Background()

	if err := runStore.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	park := func(ctx context.Context) error {
		if err := interrupts.Open(ctx, pendingForRun(
			"run_1", "ses_A", "member_1", approvalInterrupts(), time.Unix(2, 0).UTC(),
		)); err != nil {
			return err
		}
		return suspendRun(ctx, runStore, parkedRun("run_1", "ses_A"), "seg_open")
	}
	if err := sqlite.RunInTx(ctx, db, park); err != nil {
		t.Fatalf("park commit: %v", err)
	}
	if _, found, readErr := runStore.Run(ctx, "run_1"); readErr != nil || !found {
		t.Fatalf("parked Run read = (found %t, %v), want a healthy waiting Run", found, readErr)
	}

	if _, execErr := db.ExecContext(ctx,
		`UPDATE interrupts SET payload = ? WHERE root_run_id = ?`, "{not json", "run_1",
	); execErr != nil {
		t.Fatalf("corrupt payload: %v", execErr)
	}

	_, _, readErr := runStore.Run(ctx, "run_1")
	if readErr == nil {
		t.Fatal("waiting Run read accepted a corrupt pending set")
	}
	if !strings.Contains(readErr.Error(), "run_1") {
		t.Fatalf("read error = %v, want it to name the unusable Run", readErr)
	}
}
