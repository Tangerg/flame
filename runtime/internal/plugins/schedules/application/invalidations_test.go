package schedules

import (
	"context"
	"errors"
	"testing"
	"time"

	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
)

var errScheduleMutation = errors.New("schedule mutation failed")
var testScheduleCreatedAt = time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

type invalidationScheduleStore struct {
	*runNowStore
	fail        string
	deleteFound bool
}

func (i *invalidationScheduleStore) Insert(_ context.Context, scheduled schedule.Schedule) error {
	if i.fail == "create" {
		return errScheduleMutation
	}
	i.created = scheduled
	return nil
}

func (i *invalidationScheduleStore) Update(_ context.Context, _ schedule.Replacement) error {
	if i.fail == "update" {
		return errScheduleMutation
	}
	return nil
}

func (i *invalidationScheduleStore) Delete(context.Context, string) (bool, error) {
	if i.fail == "delete" {
		return false, errScheduleMutation
	}
	return i.deleteFound, nil
}

func TestCommittedScheduleMutationsSignalChanges(t *testing.T) {
	store := &invalidationScheduleStore{
		runNowStore: &runNowStore{schedule: mustStoredSchedule(t, schedule.Snapshot{
			ID: "sch_updated", Revision: 1, Instructions: "before", Cron: "@daily",
			CreatedAt: testScheduleCreatedAt, NextRunAt: testScheduleCreatedAt.Add(time.Hour),
		})},
		deleteFound: true,
	}
	var changes int
	coordinator := mustCoordinator(t, Dependencies{
		Store:         store,
		Models:        allowModels{},
		NewScheduleID: fixedIdentity("sch_created"),
		OnChange: func() {
			changes++
		},
	})

	if _, err := coordinator.Create(t.Context(), schedule.Draft{
		Instructions: "create", Cron: "@daily",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	title := "after"
	if _, err := coordinator.Update(t.Context(), UpdateCommand{
		ID: "sch_updated", ExpectedRevision: 1, Patch: Patch{Title: &title},
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := coordinator.Delete(t.Context(), "sch_deleted"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	want := 3
	if changes != want {
		t.Fatalf("changes = %d, want %d", changes, want)
	}
}

func TestScheduleMutationsPublishOnlyAfterActualCommit(t *testing.T) {
	for _, operation := range []string{"create", "update", "delete"} {
		t.Run(operation, func(t *testing.T) {
			store := &invalidationScheduleStore{
				runNowStore: &runNowStore{schedule: mustStoredSchedule(t, schedule.Snapshot{
					ID: "sch_1", Revision: 1, Instructions: "before", Cron: "@daily",
					CreatedAt: testScheduleCreatedAt, NextRunAt: testScheduleCreatedAt.Add(time.Hour),
				})},
				fail: operation, deleteFound: true,
			}
			var changes int
			coordinator := mustCoordinator(t, Dependencies{Store: store, Models: allowModels{}, NewScheduleID: fixedIdentity("sch_created"), OnChange: func() {
				changes++
			}})

			var err error
			switch operation {
			case "create":
				_, err = coordinator.Create(t.Context(), schedule.Draft{Instructions: "create", Cron: "@daily", Enabled: true})
			case "update":
				title := "after"
				_, err = coordinator.Update(t.Context(), UpdateCommand{
					ID: "sch_1", ExpectedRevision: 1, Patch: Patch{Title: &title},
				})
			case "delete":
				err = coordinator.Delete(t.Context(), "sch_1")
			}
			if !errors.Is(err, errScheduleMutation) {
				t.Fatalf("error = %v, want %v", err, errScheduleMutation)
			}
			if changes != 0 {
				t.Fatalf("published after failed %s: %+v", operation, changes)
			}
		})
	}

	store := &invalidationScheduleStore{runNowStore: &runNowStore{}, deleteFound: false}
	var changes int
	coordinator := mustCoordinator(t, Dependencies{Store: store, Models: allowModels{}, OnChange: func() {
		changes++
	}})
	if err := coordinator.Delete(t.Context(), "sch_missing"); err != nil {
		t.Fatalf("idempotent Delete: %v", err)
	}
	if changes != 0 {
		t.Fatalf("idempotent delete published a false mutation: %+v", changes)
	}
}
