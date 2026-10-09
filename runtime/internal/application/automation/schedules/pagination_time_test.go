package schedules

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
)

func TestListPagePreservesScheduleMillisecondCoordinates(t *testing.T) {
	for _, at := range []time.Time{time.Date(1000, 1, 1, 0, 0, 0, 0, time.UTC), time.Unix(0, 0).UTC(), time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC)} {
		t.Run(at.Format(time.RFC3339), func(t *testing.T) {
			store := &pagedStore{runNowStore: &runNowStore{}, rows: []schedule.Schedule{
				mustStoredSchedule(t, schedule.Snapshot{ID: "sch_first", Instructions: "first", CreatedAt: at}),
				mustStoredSchedule(t, schedule.Snapshot{ID: "sch_second", Instructions: "second", CreatedAt: at.Add(-time.Millisecond)}),
			}}
			coordinator := mustCoordinator(t, Dependencies{Store: store, Models: allowModels{}})
			first, err := coordinator.ListPage(t.Context(), "", explicitPageLimit(t, 1))
			if err != nil || len(first.Rows) != 1 || first.Rows[0].ID() != "sch_first" || first.NextCursor == "" {
				t.Fatalf("first page = %+v, %v", first, err)
			}
			second, err := coordinator.ListPage(t.Context(), first.NextCursor, explicitPageLimit(t, 1))
			if err != nil || len(second.Rows) != 1 || second.Rows[0].ID() != "sch_second" || second.NextCursor != "" {
				t.Fatalf("second page = %+v, %v; want sch_second without another page", second, err)
			}
		})
	}
}

func TestListPageRefusesFormerNanosecondCoordinates(t *testing.T) {
	store := &pagedStore{runNowStore: &runNowStore{}, rows: scheduleRows(t, "sch_first", "sch_second")}
	coordinator := mustCoordinator(t, Dependencies{Store: store, Models: allowModels{}})
	cursor, err := pagination.Encode("schedules", nil, []string{"1000000", "sch_first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.ListPage(t.Context(), cursor, explicitPageLimit(t, 1)); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("former nanosecond cursor = %v, want ErrInvalidCursor", err)
	}
}
