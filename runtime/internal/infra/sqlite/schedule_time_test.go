package sqlite_test

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
)

func TestScheduleRoundTripDistinguishesEpochFromAbsentTime(t *testing.T) {
	store := newScheduleStore(t)
	scheduled, err := schedule.New("sch_epoch_manual", schedule.Draft{
		Instructions: "review", Cron: "@hourly",
	}, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Insert(t.Context(), scheduled); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRun(t.Context(), testRunRecord(scheduled, scheduled.CreatedAt())); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(t.Context(), scheduled.ID())
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastRunAt().Equal(scheduled.CreatedAt()) || !got.NextRunAt().IsZero() || got.Enabled() {
		t.Fatalf("restored manual Schedule = %+v, want recorded epoch and disabled cursor", got.Snapshot())
	}
}

func TestScheduleDueAndPendingPreserveEpochBoundary(t *testing.T) {
	store, runStore, _ := newScheduleRunStores(t)
	for _, test := range []struct {
		id    string
		dueAt time.Time
	}{
		{"sch_before_epoch", time.Unix(-3600, 0).UTC()},
		{"sch_at_epoch", time.Unix(0, 0).UTC()},
	} {
		scheduled, err := schedule.New(test.id, schedule.Draft{
			Instructions: "review", Cron: "@hourly", Enabled: true,
		}, test.dueAt.Add(-time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if !scheduled.NextRunAt().Equal(test.dueAt) {
			t.Fatalf("initial cursor = %v, want %v", scheduled.NextRunAt(), test.dueAt)
		}
		if err := store.Insert(t.Context(), scheduled); err != nil {
			t.Fatal(err)
		}
		got, err := store.Get(t.Context(), scheduled.ID())
		if err != nil || !got.Enabled() || !got.NextRunAt().Equal(test.dueAt) {
			t.Fatalf("restored Schedule = %+v, %v, want enabled cursor %v", got.Snapshot(), err, test.dueAt)
		}
		due, err := store.Due(t.Context(), test.dueAt, 1)
		if err != nil || len(due) != 1 || due[0].ID() != scheduled.ID() {
			t.Fatalf("Due = %+v, %v, want %s", due, err, scheduled.ID())
		}
		claim, err := scheduled.Claim("ses_"+test.id, "run_"+test.id, test.dueAt)
		if err != nil {
			t.Fatal(err)
		}
		if claimed, err := store.Claim(t.Context(), claim); err != nil || !claimed {
			t.Fatalf("Claim = %t, %v", claimed, err)
		}
	}
	first, err := store.Pending(t.Context(), time.Time{}, "", 1)
	if err != nil || len(first) != 1 || first[0].ScheduleID() != "sch_before_epoch" {
		t.Fatalf("first Pending page = %+v, %v", first, err)
	}
	second, err := store.Pending(t.Context(), first[0].DueAt(), first[0].ID(), 1)
	if err != nil || len(second) != 1 || second[0].ScheduleID() != "sch_at_epoch" {
		t.Fatalf("second Pending page = %+v, %v", second, err)
	}
	for _, occurrence := range append(first, second...) {
		if !occurrence.DueAt().Equal(occurrence.FiredAt()) {
			t.Fatalf("restored firing time = %v, want %v", occurrence.FiredAt(), occurrence.DueAt())
		}
		admitOccurrenceRun(t, runStore, occurrence)
		if err := store.Accept(t.Context(), testAcceptance(occurrence)); err != nil {
			t.Fatal(err)
		}
		got, err := store.Get(t.Context(), occurrence.ScheduleID())
		if err != nil || !got.LastRunAt().Equal(occurrence.FiredAt()) || !got.NextRunAt().Equal(occurrence.NextRunAt()) {
			t.Fatalf("accepted Schedule = %+v, %v", got.Snapshot(), err)
		}
	}
}
