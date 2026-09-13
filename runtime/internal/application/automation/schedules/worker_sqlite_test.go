package schedules

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type sqliteScheduledStarter struct {
	store     *sqlite.ScheduleStore
	runs      *sqlite.RunStore
	now       time.Time
	attempted []string
}

func (s *sqliteScheduledStarter) StartScheduledRun(ctx context.Context, request schedule.RunRequest) error {
	s.attempted = append(s.attempted, request.ScheduleID())
	if strings.HasPrefix(request.ScheduleID(), "sch_failed") {
		return errors.New("provider unavailable")
	}
	if err := s.runs.Admit(ctx, run.Draft{RunID: request.RunID(), SessionID: request.SessionID(), SegmentID: "seg_test", ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: s.now}); err != nil {
		return err
	}
	acceptance, err := schedule.NewAcceptance(request.OccurrenceID(), request.RunID())
	if err != nil {
		return err
	}
	return s.store.Accept(ctx, acceptance)
}

func TestWorkerAdvancesHealthySchedulesPastFailedSQLiteBacklog(t *testing.T) {
	captureWorkerDiagnostics(t)
	ctx := t.Context()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "schedules.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := sqlite.NewScheduleStore(db)
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	for i := range workerBatchSize + 1 {
		id := fmt.Sprintf("sch_failed_%02d", i)
		if i == workerBatchSize {
			id = "sch_healthy_pending"
		}
		scheduled := dueSchedule(t, id, now.Add(-2*time.Minute))
		if err := store.Insert(ctx, scheduled); err != nil {
			t.Fatal(err)
		}
		claim, err := schedule.NewClaim(scheduled, "ses_"+id, "run_"+id, now.Add(-2*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if claimed, err := store.Claim(ctx, claim); err != nil || !claimed {
			t.Fatalf("claim %s: %v, %v", id, claimed, err)
		}
	}
	if err := store.Insert(ctx, dueSchedule(t, "sch_healthy_new", now)); err != nil {
		t.Fatal(err)
	}
	starter := &sqliteScheduledStarter{store: store, runs: sqlite.NewRunStore(db), now: now}
	sequence := 0
	identity := func() string { sequence++; return fmt.Sprintf("%d", sequence) }
	worker := newWorker(workerDependencies{Store: store, RunStarter: starter, NewSessionID: func() string { return "ses_" + identity() }, NewRunID: func() string { return "run_" + identity() }})
	worker.fireDue(ctx, now)
	healthy, err := store.Get(ctx, "sch_healthy_new")
	if err != nil || healthy.LastRunAt().IsZero() {
		t.Fatalf("new healthy schedule starved: %v, %v", healthy, err)
	}
	for range 3 {
		before := len(starter.attempted)
		worker.fireDue(ctx, now)
		if attempts := len(starter.attempted) - before; attempts > workerBatchSize {
			t.Fatalf("pass attempted %d runs", attempts)
		}
	}
	healthy, err = store.Get(ctx, "sch_healthy_pending")
	if err != nil || healthy.LastRunAt().IsZero() {
		t.Fatalf("pending healthy occurrence starved: %v, %v", healthy, err)
	}
}
