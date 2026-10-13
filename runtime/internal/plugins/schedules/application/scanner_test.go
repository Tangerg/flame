package schedules_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"path/filepath"
	"testing"
	"time"

	app "github.com/Tangerg/flame/runtime/internal/plugins/schedules/application"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/sqlite"
)

func TestScannerRotatesBacklogAndPreservesCapturedWorkAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedules.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	store := sqlite.NewScheduleStore(db)
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	create := func(id string, created time.Time, enabled bool) schedule.Schedule {
		value, err := schedule.New(id, schedule.Draft{Title: id, Instructions: "captured instructions", CWD: "/project", Cron: "@hourly", Enabled: enabled}, created)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Insert(t.Context(), value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	for i := range 40 {
		value := create(fmt.Sprintf("sch_backlog_%02d", i), now.Add(-time.Hour-time.Second), true)
		claim, err := value.Claim(fmt.Sprintf("ses_backlog_%02d", i), fmt.Sprintf("run_backlog_%02d", i), now.Add(-time.Hour), testsupport.ScheduleInvocation())
		if err != nil {
			t.Fatal(err)
		}
		if accepted, err := store.Claim(t.Context(), claim); err != nil || !accepted {
			t.Fatalf("claim=%t %v", accepted, err)
		}
	}
	manual := create("sch_manual", now.Add(-time.Hour), false)
	for i := range 20 {
		request, err := manual.ManualRun(fmt.Sprintf("ses_manual_%02d", i), fmt.Sprintf("run_manual_%02d", i), now, testsupport.ScheduleInvocation())
		if err != nil {
			t.Fatal(err)
		}
		if err := store.QueueManual(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Delete(t.Context(), manual.ID()); err != nil {
		t.Fatal(err)
	}
	create("sch_new_due", now.Add(-time.Second), true)
	sequence := 0
	factory := func(prefix string) func() string {
		return func() string { sequence++; return fmt.Sprintf("%snew_%d", prefix, sequence) }
	}
	scanner, err := app.NewScanner(store, store, factory("ses_"), factory("run_"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]schedule.RunRequestSnapshot{}
	newDue := false
	for pass := range 4 {
		requests, err := scanner.Scan(t.Context(), now, testsupport.ScheduleInvocation())
		if err != nil {
			t.Fatal(err)
		}
		if len(requests) > schedule.MaximumDispatchBatch {
			t.Fatal("scan exceeded dispatch bound")
		}
		batch := map[string]bool{}
		for _, request := range requests {
			if batch[request.RunID()] {
				t.Fatalf("duplicate Run in batch: %s", request.RunID())
			}
			batch[request.RunID()] = true
			seen[request.RunID()] = request.Snapshot()
			if request.ScheduleID() == "sch_new_due" {
				newDue = true
			}
		}
		if pass == 0 && !newDue {
			t.Fatal("pending backlog starved newly due work")
		}
	}
	for i := range 20 {
		if _, ok := seen[fmt.Sprintf("run_manual_%02d", i)]; !ok {
			t.Fatal("manual backlog failed to rotate")
		}
	}
	for i := range 40 {
		if _, ok := seen[fmt.Sprintf("run_backlog_%02d", i)]; !ok {
			t.Fatal("cron backlog failed to rotate")
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	store = sqlite.NewScheduleStore(db)
	pending, err := store.PendingManual(t.Context(), "", schedule.MaximumDispatchBatch)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 20 {
		t.Fatalf("restart lost captured manual requests: %d", len(pending))
	}
	for _, request := range pending {
		if request.Snapshot() != seen[request.RunID()] {
			t.Fatal("restart changed captured identity or execution")
		}
	}
	if err := store.AcceptRun(t.Context(), pending[0].RunID()); err != nil {
		t.Fatal(err)
	}
	if err := store.AcceptRun(t.Context(), pending[0].RunID()); err != nil {
		t.Fatal(err)
	}
	pending, err = store.PendingManual(t.Context(), "", schedule.MaximumDispatchBatch)
	if err != nil || len(pending) != 19 {
		t.Fatalf("acceptance replay=%d %v", len(pending), err)
	}
}
func TestCanceledScannerDoesNotClaimDueWork(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "schedules.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := sqlite.NewScheduleStore(db)
	calls := 0
	identity := func() string { calls++; return "unexpected" }
	scanner, err := app.NewScanner(store, store, identity, identity)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := scanner.Scan(ctx, time.Now(), testsupport.ScheduleInvocation()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled scan=%v", err)
	}
	if calls != 0 {
		t.Fatal("canceled scan allocated dispatch identities")
	}
}
