package sqlite

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	sqlitedriver "modernc.org/sqlite"
	sqlite3lib "modernc.org/sqlite/lib"
)

func TestScheduleRevisionRequiresExactPositiveInteger(t *testing.T) {
	for _, test := range []struct {
		revision uint64
		rejected bool
	}{
		{1, false}, {exactint.Maximum, false}, {0, true}, {exactint.Maximum + 1, true},
	} {
		t.Run(fmt.Sprint(test.revision), func(t *testing.T) {
			store, db := newScheduleStores(t)
			dueAt := time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
			created, err := insertSchedule(t.Context(), store, schedule.Snapshot{
				ID:           schedule.IDPrefix + "00000000-0000-4000-8000-000000000001",
				Instructions: "review", Cron: "0 * * * *", CreatedAt: dueAt.Add(-time.Hour), NextRunAt: dueAt,
			})
			if err != nil {
				t.Fatalf("seed valid Schedule: %v", err)
			}
			result, err := db.ExecContext(t.Context(), `UPDATE schedules SET revision = ? WHERE id = ?`, test.revision, created.ID())
			want := test.revision
			if test.rejected {
				failure, ok := errors.AsType[*sqlitedriver.Error](err)
				if !ok || failure.Code() != sqlite3lib.SQLITE_CONSTRAINT_CHECK {
					t.Fatalf("revision %d = %v, want SQLite CHECK refusal", test.revision, err)
				}
				want = created.Revision()
			} else {
				if err != nil {
					t.Fatalf("valid revision %d: %v", test.revision, err)
				}
				if changed, err := result.RowsAffected(); err != nil || changed != 1 {
					t.Fatalf("valid revision changed %d rows: %v", changed, err)
				}
			}
			retained, err := store.Get(t.Context(), created.ID())
			if err != nil || retained.Revision() != want {
				t.Fatalf("retained revision = %d, %v; want %d", retained.Revision(), err, want)
			}
		})
	}
}
