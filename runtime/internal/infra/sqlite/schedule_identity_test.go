package sqlite_test

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
)

func TestScheduleOccurrenceIndexesCannotRewriteItsIdentity(t *testing.T) {
	store, _, db := newScheduleRunStores(t)
	dueAt := time.Unix(3600, 0).UTC()
	scheduled, err := schedule.New("sch_identity", schedule.Draft{
		Instructions: "review", Cron: "@hourly", Enabled: true,
	}, dueAt.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Insert(t.Context(), scheduled); err != nil {
		t.Fatal(err)
	}
	claim := testClaim(scheduled, "ses_identity", "run_identity", dueAt)
	if claimed, err := store.Claim(t.Context(), claim); err != nil || !claimed {
		t.Fatalf("Claim = %t, %v", claimed, err)
	}
	for _, statement := range []string{
		`UPDATE schedule_firings SET schedule_id = 'sch_other'`,
		`UPDATE schedule_firings SET due_at = due_at + 1`,
		`UPDATE schedule_firings SET id = 'sch_other:3600000'`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err == nil {
			t.Fatalf("storage permitted an occurrence projection to rewrite identity: %s", statement)
		}
	}
	pending, err := store.Pending(t.Context(), time.Time{}, "", 1)
	if err != nil || len(pending) != 1 || pending[0].ID() != claim.Occurrence().ID() {
		t.Fatalf("rejected identity writes changed pending work: %+v, %v", pending, err)
	}
}
