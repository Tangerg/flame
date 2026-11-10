package schedules

import (
	"context"
	"errors"
	"testing"
	"time"

	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
)

var errScheduleMutation = errors.New("schedule mutation failed")
var testScheduleCreatedAt = time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

type mutationScheduleStore struct {
	*runNowStore
	fail        string
	deleteFound bool
}

func (i *mutationScheduleStore) Insert(_ context.Context, scheduled schedule.Schedule) error {
	if i.fail == "create" {
		return errScheduleMutation
	}
	i.created = scheduled
	return nil
}

func (i *mutationScheduleStore) Update(_ context.Context, _ schedule.Replacement) error {
	if i.fail == "update" {
		return errScheduleMutation
	}
	return nil
}

func (i *mutationScheduleStore) Delete(context.Context, string) (bool, error) {
	if i.fail == "delete" {
		return false, errScheduleMutation
	}
	return i.deleteFound, nil
}

func TestSchedulePreparationDefersAllStoreMutations(t *testing.T) {
	for _, operation := range []string{"create", "update", "delete"} {
		t.Run(operation, func(t *testing.T) {
			store := &mutationScheduleStore{
				runNowStore: &runNowStore{schedule: mustStoredSchedule(t, schedule.Snapshot{
					ID: "sch_1", Revision: 1, Instructions: "before", Cron: "0 0 * * *", TimeZone: "UTC",
					CreatedAt: testScheduleCreatedAt, NextRunAt: testScheduleCreatedAt.Add(time.Hour),
				})},
				fail: operation, deleteFound: true,
			}
			coordinator := mustCoordinator(t, Dependencies{Store: store, NewScheduleID: fixedIdentity("sch_created")})
			var apply func(context.Context) error
			var err error
			switch operation {
			case "create":
				var mutation ScheduleMutation
				mutation, err = coordinator.PrepareCreate(t.Context(), schedule.Draft{Instructions: "create", Cron: "0 0 * * *", TimeZone: "UTC", Enabled: true})
				apply = mutation.Apply
			case "update":
				var mutation ScheduleMutation
				mutation, err = coordinator.PrepareUpdate(t.Context(), UpdateCommand{ID: "sch_1", ExpectedRevision: 1, Patch: Patch{Title: new("after")}})
				apply = mutation.Apply
			case "delete":
				apply, err = coordinator.PrepareDelete(t.Context(), "sch_1")
			}
			if err != nil {
				t.Fatalf("preparation applied its write: %v", err)
			}
			if err := apply(t.Context()); !errors.Is(err, errScheduleMutation) {
				t.Fatalf("write failure = %v", err)
			}
		})
	}
}

func TestDeleteMissingScheduleHasNoWritePlan(t *testing.T) {
	store := &mutationScheduleStore{runNowStore: &runNowStore{}, fail: "delete"}
	coordinator := mustCoordinator(t, Dependencies{Store: store})
	apply, err := coordinator.PrepareDelete(t.Context(), "sch_missing")
	if err != nil || apply != nil {
		t.Fatalf("idempotent removal created a write: apply=%v, err=%v", apply != nil, err)
	}
}
