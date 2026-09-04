package schedules

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
)

type workerStore struct {
	due                []schedule.Schedule
	dueErr             error
	claims             []claimRecord
	claimContextErrors []error
	pending            []schedule.Occurrence
	claimed            map[string]bool
}

type claimRecord struct {
	id            string
	ranAt         time.Time
	prevNextRunAt time.Time
	nextRunAt     time.Time
}

func (w *workerStore) Get(context.Context, string) (schedule.Schedule, error) {
	return schedule.Schedule{}, schedule.ErrNotFound
}
func (w *workerStore) Insert(context.Context, schedule.Schedule) error { return nil }
func (w *workerStore) Update(context.Context, schedule.Schedule, uint64) (schedule.Schedule, error) {
	return schedule.Schedule{}, nil
}
func (w *workerStore) Delete(context.Context, string) (bool, error) { return false, nil }
func (w *workerStore) Due(_ context.Context, _ time.Time, _ int) ([]schedule.Schedule, error) {
	return w.due, w.dueErr
}
func (w *workerStore) Claim(ctx context.Context, claim schedule.Claim) (bool, error) {
	occurrence := claim.Occurrence()
	if w.claimed == nil {
		w.claimed = map[string]bool{}
	}
	if w.claimed[occurrence.ID()] {
		return false, nil
	}
	w.claimed[occurrence.ID()] = true
	w.claims = append(w.claims, claimRecord{id: occurrence.ScheduleID(), ranAt: occurrence.FiredAt(), prevNextRunAt: occurrence.DueAt(), nextRunAt: occurrence.NextRunAt()})
	w.claimContextErrors = append(w.claimContextErrors, ctx.Err())
	w.pending = append(w.pending, occurrence)
	return true, nil
}
func (w *workerStore) Pending(context.Context, int) ([]schedule.Occurrence, error) {
	return w.pending, nil
}
func (w *workerStore) RecordRun(context.Context, schedule.RunRecord) error { return nil }

type recordingScheduledRunStarter struct {
	startErr           error
	startedScheduleIDs []string
}

func (r *recordingScheduledRunStarter) StartScheduledRun(_ context.Context, request schedule.RunRequest) (StartedRun, error) {
	r.startedScheduleIDs = append(r.startedScheduleIDs, request.ScheduleID())
	if r.startErr != nil {
		return StartedRun{}, r.startErr
	}
	return StartedRun{SessionID: "ses_1", RunID: "run_1"}, nil
}

func TestWorkerFireDueLeavesFailedOccurrenceDue(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	prev := now.Add(-time.Minute)
	store := &workerStore{due: []schedule.Schedule{dueSchedule(t, "sch_1", prev)}}
	runner := &recordingScheduledRunStarter{startErr: errors.New("boom")}
	var notices []invalidation.Notice
	w := newWorker(workerDependencies{
		Store: store, RunStarter: runner, NewSessionID: fixedSessionID, NewRunID: fixedRunID,
		Invalidations: func(notice invalidation.Notice) { notices = append(notices, notice) },
	})

	// The durable due row is intentionally presented again on the next scan: a
	// rejected run must never be recorded as fired, even after a process restart.
	w.fireDue(context.Background(), now)
	w.fireDue(context.Background(), now)
	if len(runner.startedScheduleIDs) != 2 {
		t.Fatalf("started = %d, want 2", len(runner.startedScheduleIDs))
	}
	if len(store.claims) != 1 {
		t.Fatalf("claims = %d, want one durable occurrence", len(store.claims))
	}
	if len(notices) != 1 || !slices.Equal(notices[0].ScheduleIDs, []string{"sch_1"}) {
		t.Fatalf("claim invalidations = %+v, want one cursor change", notices)
	}
}

func TestWorkerFireDueRejectsAnInvalidAggregate(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &workerStore{due: []schedule.Schedule{dueSchedule(t, "sch_valid", now), {}}}
	runner := &recordingScheduledRunStarter{}

	newWorker(workerDependencies{Store: store, RunStarter: runner, NewSessionID: fixedSessionID, NewRunID: fixedRunID}).fireDue(context.Background(), now)

	if len(runner.startedScheduleIDs) != 0 {
		t.Fatalf("started = %d, want 0", len(runner.startedScheduleIDs))
	}
	if len(store.claims) != 0 {
		t.Fatalf("claims = %+v, want none", store.claims)
	}
}

func TestWorkerFireDueValidatesCompletePendingBatchBeforeDispatch(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &workerStore{pending: []schedule.Occurrence{
		pendingOccurrence(t, "sch_valid", now.Add(-time.Minute)),
		{},
	}}
	runner := &recordingScheduledRunStarter{}

	newWorker(workerDependencies{
		Store: store, RunStarter: runner, NewSessionID: fixedSessionID, NewRunID: fixedRunID,
	}).fireDue(t.Context(), now)

	if len(runner.startedScheduleIDs) != 0 {
		t.Fatalf("started = %v, want none", runner.startedScheduleIDs)
	}
}

func TestValidatePendingBatchRejectsBrokenStoreResults(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	first := pendingOccurrence(t, "sch_1", now.Add(-2*time.Minute))
	second := pendingOccurrence(t, "sch_2", now.Add(-time.Minute))
	sameSchedule := pendingOccurrence(t, "sch_1", now.Add(-time.Minute))
	for name, test := range map[string]struct {
		rows    []schedule.Occurrence
		maximum int
	}{
		"excess rows":          {rows: []schedule.Occurrence{first, second}, maximum: 1},
		"invalid after valid":  {rows: []schedule.Occurrence{first, {}}, maximum: 2},
		"duplicate occurrence": {rows: []schedule.Occurrence{first, first}, maximum: 2},
		"duplicate Schedule":   {rows: []schedule.Occurrence{first, sameSchedule}, maximum: 2},
		"out of order":         {rows: []schedule.Occurrence{second, first}, maximum: 2},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validatePendingBatch(test.rows, test.maximum); err == nil {
				t.Fatal("validatePendingBatch accepted a broken store result")
			}
		})
	}
}

func TestValidateDueBatchRejectsBrokenStoreResults(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	first := dueSchedule(t, "sch_1", now.Add(-2*time.Minute))
	second := dueSchedule(t, "sch_2", now.Add(-time.Minute))
	for name, test := range map[string]struct {
		rows    []schedule.Schedule
		maximum int
	}{
		"excess rows":         {rows: []schedule.Schedule{first, second}, maximum: 1},
		"invalid after valid": {rows: []schedule.Schedule{first, {}}, maximum: 2},
		"duplicate Schedule":  {rows: []schedule.Schedule{first, first}, maximum: 2},
		"not due":             {rows: []schedule.Schedule{dueSchedule(t, "sch_future", now.Add(time.Minute))}, maximum: 1},
		"cursor out of order": {rows: []schedule.Schedule{second, first}, maximum: 2},
		"id tie out of order": {rows: []schedule.Schedule{
			dueSchedule(t, "sch_b", now), dueSchedule(t, "sch_a", now),
		}, maximum: 2},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDueBatch(test.rows, now, test.maximum); err == nil {
				t.Fatal("validateDueBatch accepted a broken store result")
			}
		})
	}
}

func TestWorkerFireDueStopsOnDueError(t *testing.T) {
	store := &workerStore{dueErr: errors.New("db down")}
	runner := &recordingScheduledRunStarter{}

	newWorker(workerDependencies{Store: store, RunStarter: runner, NewSessionID: fixedSessionID, NewRunID: fixedRunID}).fireDue(context.Background(), time.Now())

	if len(runner.startedScheduleIDs) != 0 || len(store.claims) != 0 {
		t.Fatalf("started=%d claims=%d, want none", len(runner.startedScheduleIDs), len(store.claims))
	}
}

func TestWorkerFireDueDoesNotConsumeCancellationAbortedFiring(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &workerStore{due: []schedule.Schedule{
		dueSchedule(t, "sch_1", now),
		dueSchedule(t, "sch_2", now),
	}}
	ctx, cancel := context.WithCancel(context.Background())
	runner := cancelingScheduledRunStarter{cancel: cancel, succeed: false}

	newWorker(workerDependencies{Store: store, RunStarter: &runner, NewSessionID: fixedSessionID, NewRunID: fixedRunID}).fireDue(ctx, now)

	if len(runner.startedScheduleIDs) != 1 || runner.startedScheduleIDs[0] != "sch_1" {
		t.Fatalf("started = %v, want only sch_1", runner.startedScheduleIDs)
	}
	if len(store.claims) != 1 {
		t.Fatalf("claims = %+v, want only the occurrence dispatched before cancellation", store.claims)
	}
}

func TestWorkerFireDuePersistsAcceptedFiringAfterCancellation(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &workerStore{due: []schedule.Schedule{
		dueSchedule(t, "sch_1", now),
		dueSchedule(t, "sch_2", now),
	}}
	ctx, cancel := context.WithCancel(context.Background())
	runner := cancelingScheduledRunStarter{cancel: cancel, succeed: true}

	newWorker(workerDependencies{Store: store, RunStarter: &runner, NewSessionID: fixedSessionID, NewRunID: fixedRunID}).fireDue(ctx, now)

	if len(runner.startedScheduleIDs) != 1 || runner.startedScheduleIDs[0] != "sch_1" {
		t.Fatalf("started = %v, want only sch_1", runner.startedScheduleIDs)
	}
	if len(store.claims) != 1 || store.claims[0].id != "sch_1" {
		t.Fatalf("claims = %+v, want only sch_1", store.claims)
	}
	if len(store.claimContextErrors) != 1 || store.claimContextErrors[0] != nil {
		t.Fatalf("claim context errors = %v, want live context", store.claimContextErrors)
	}
}

func TestWorkerRunScansImmediately(t *testing.T) {
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	store := &workerStore{due: []schedule.Schedule{dueSchedule(t, "sch_1", now)}}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	runner := cancelingScheduledRunStarter{cancel: cancel, succeed: true}
	worker := newWorker(workerDependencies{Store: store, RunStarter: &runner, NewSessionID: fixedSessionID, NewRunID: fixedRunID})
	worker.now = func() time.Time { return now }

	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after its initial scan")
	}
	if len(runner.startedScheduleIDs) != 1 || runner.startedScheduleIDs[0] != "sch_1" {
		t.Fatalf("initial scan started = %v, want [sch_1]", runner.startedScheduleIDs)
	}
}

type cancelingScheduledRunStarter struct {
	cancel             context.CancelFunc
	succeed            bool
	startedScheduleIDs []string
	requests           []schedule.RunRequest
}

func (c *cancelingScheduledRunStarter) StartScheduledRun(ctx context.Context, request schedule.RunRequest) (StartedRun, error) {
	c.startedScheduleIDs = append(c.startedScheduleIDs, request.ScheduleID())
	c.requests = append(c.requests, request)
	c.cancel()
	if !c.succeed {
		return StartedRun{}, ctx.Err()
	}
	return StartedRun{SessionID: "ses_1", RunID: "run_1"}, nil
}

func dueSchedule(t testing.TB, id string, dueAt time.Time) schedule.Schedule {
	t.Helper()
	return mustStoredSchedule(t, schedule.Snapshot{
		ID: id, Instructions: "review", Cron: "* * * * *", Enabled: true,
		CreatedAt: dueAt.Add(-time.Hour), NextRunAt: dueAt, Revision: 1,
	})
}

func pendingOccurrence(t testing.TB, scheduleID string, dueAt time.Time) schedule.Occurrence {
	t.Helper()
	claim, err := schedule.NewClaim(
		dueSchedule(t, scheduleID, dueAt),
		"ses_"+scheduleID,
		"run_"+scheduleID+"_"+dueAt.Format("150405"),
		dueAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	return claim.Occurrence()
}
