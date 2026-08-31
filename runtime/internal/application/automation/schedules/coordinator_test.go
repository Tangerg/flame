package schedules

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

type allowModels struct{}

func (allowModels) AdmitSelection(modelref.Selection) error { return nil }

func fixedSessionID() string  { return "ses_schedule" }
func fixedRunID() string      { return "run_schedule" }
func fixedScheduleID() string { return "sch_test" }

func fixedIdentity(value string) func() string {
	return func() string { return value }
}

type identityCWDResolver struct{}

func (identityCWDResolver) ResolveExistingDir(path string) (string, error) { return path, nil }

func mustCoordinator(t testing.TB, deps Dependencies) *Coordinator {
	t.Helper()
	if deps.Paths == nil {
		deps.Paths = identityCWDResolver{}
	}
	if deps.NewScheduleID == nil {
		deps.NewScheduleID = fixedScheduleID
	}
	value, err := New(deps)
	if err != nil {
		t.Fatalf("New Coordinator: %v", err)
	}
	return value
}

func mustFiring(t testing.TB, deps FiringDependencies) *Firing {
	t.Helper()
	if deps.NewSessionID == nil {
		deps.NewSessionID = fixedSessionID
	}
	if deps.NewRunID == nil {
		deps.NewRunID = fixedRunID
	}
	value, err := NewFiring(deps)
	if err != nil {
		t.Fatalf("NewFiring: %v", err)
	}
	return value
}

// TestNilRegistryDisablesCRUD: a coordinator built without a store reports
// every CRUD op as unavailable (the no-scheduling build), rather than panicking.
func TestNilRegistryDisablesCRUD(t *testing.T) {
	c := Disabled()
	ctx := context.Background()

	if c.Available() {
		t.Fatal("Available = true, want false")
	}
	if _, err := c.ListPage(ctx, "", explicitPageLimit(t, 1)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ListPage err = %v, want ErrUnavailable", err)
	}
	if _, err := c.Create(ctx, CreateCommand{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Create err = %v, want ErrUnavailable", err)
	}
	if _, err := c.Update(ctx, UpdateCommand{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Update err = %v, want ErrUnavailable", err)
	}
	if err := c.Delete(ctx, "sch_1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Delete err = %v, want ErrUnavailable", err)
	}
	firing := DisabledFiring()
	if firing.Available() {
		t.Fatal("firing Available = true, want false")
	}
	if _, err := firing.RunNow(ctx, "sch_1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("RunNow err = %v, want ErrUnavailable", err)
	}
}

func TestScheduleConstructorsRejectPartialAndTypedNilDependencies(t *testing.T) {
	store := &runNowStore{}
	runner := &cancelingScheduledRunStarter{cancel: func() {}, succeed: true}
	coordinatorCases := []struct {
		name string
		deps Dependencies
	}{
		{name: "missing store", deps: Dependencies{Paths: identityCWDResolver{}, Models: allowModels{}, NewScheduleID: fixedScheduleID}},
		{name: "missing paths", deps: Dependencies{Store: store, Models: allowModels{}, NewScheduleID: fixedScheduleID}},
		{name: "missing models", deps: Dependencies{Store: store, Paths: identityCWDResolver{}, NewScheduleID: fixedScheduleID}},
		{name: "missing identity", deps: Dependencies{Store: store, Paths: identityCWDResolver{}, Models: allowModels{}}},
	}
	var typedNilStore *runNowStore
	coordinatorCases = append(coordinatorCases, struct {
		name string
		deps Dependencies
	}{name: "typed nil store", deps: Dependencies{
		Store: typedNilStore, Paths: identityCWDResolver{}, Models: allowModels{}, NewScheduleID: fixedScheduleID,
	}})
	for _, test := range coordinatorCases {
		t.Run("coordinator "+test.name, func(t *testing.T) {
			if value, err := New(test.deps); err == nil || value != nil {
				t.Fatalf("New = (%v, %v), want nil construction error", value, err)
			}
		})
	}
	firingCases := []struct {
		name string
		deps FiringDependencies
	}{
		{name: "missing store", deps: FiringDependencies{RunStarter: runner, NewSessionID: fixedSessionID, NewRunID: fixedRunID}},
		{name: "missing runner", deps: FiringDependencies{Store: store, NewSessionID: fixedSessionID, NewRunID: fixedRunID}},
		{name: "missing Session identity", deps: FiringDependencies{Store: store, RunStarter: runner, NewRunID: fixedRunID}},
		{name: "missing Run identity", deps: FiringDependencies{Store: store, RunStarter: runner, NewSessionID: fixedSessionID}},
		{name: "typed nil store", deps: FiringDependencies{Store: typedNilStore, RunStarter: runner, NewSessionID: fixedSessionID, NewRunID: fixedRunID}},
	}
	for _, test := range firingCases {
		t.Run("firing "+test.name, func(t *testing.T) {
			if value, err := NewFiring(test.deps); err == nil || value != nil {
				t.Fatalf("NewFiring = (%v, %v), want nil construction error", value, err)
			}
		})
	}
}

func TestRunNowRecordsAcceptedRunAfterRequestCancellation(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	store := &runNowStore{schedule: mustStoredSchedule(t, schedule.Snapshot{ID: "sch_1", Instructions: "review"})}
	ctx, cancel := context.WithCancel(context.Background())
	runner := cancelingScheduledRunStarter{cancel: cancel, succeed: true}
	var notices []invalidation.Notice
	firing := mustFiring(t, FiringDependencies{
		Store: store, RunStarter: &runner,
		Invalidations: func(notice invalidation.Notice) { notices = append(notices, notice) },
	})
	firing.now = func() time.Time { return now }

	if _, err := firing.RunNow(ctx, "sch_1"); err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if store.recordedID != "sch_1" || !store.recordedAt.Equal(now) {
		t.Fatalf("recorded = (%q, %v), want (sch_1, %v)", store.recordedID, store.recordedAt, now)
	}
	if store.recordCtxErr != nil {
		t.Fatalf("record context error = %v, want live post-accept context", store.recordCtxErr)
	}
	if len(notices) != 1 || notices[0].Resource != invalidation.Schedules ||
		!slices.Equal(notices[0].ScheduleIDs, []string{"sch_1"}) {
		t.Fatalf("post-record invalidations = %+v, want schedules/sch_1", notices)
	}
}

func TestRunNowDoesNotRecordCancellationAbortedRun(t *testing.T) {
	store := &runNowStore{schedule: mustStoredSchedule(t, schedule.Snapshot{ID: "sch_1", Instructions: "review"})}
	ctx, cancel := context.WithCancel(context.Background())
	runner := cancelingScheduledRunStarter{cancel: cancel, succeed: false}
	firing := mustFiring(t, FiringDependencies{
		Store: store, RunStarter: &runner,
	})

	if _, err := firing.RunNow(ctx, "sch_1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunNow error = %v, want context.Canceled", err)
	}
	if store.recordedID != "" {
		t.Fatalf("recorded id = %q, want none", store.recordedID)
	}
}

func TestRunNowRecordFailureDoesNotPublishSchedule(t *testing.T) {
	store := &runNowStore{
		schedule:  mustStoredSchedule(t, schedule.Snapshot{ID: "sch_1", Instructions: "review"}),
		recordErr: errors.New("record failed"),
	}
	runner := &cancelingScheduledRunStarter{cancel: func() {}, succeed: true}
	var notices []invalidation.Notice
	firing := mustFiring(t, FiringDependencies{
		Store: store, RunStarter: runner,
		Invalidations: func(notice invalidation.Notice) { notices = append(notices, notice) },
	})

	if _, err := firing.RunNow(t.Context(), "sch_1"); err == nil {
		t.Fatal("RunNow error = nil, want record failure")
	}
	if len(notices) != 0 {
		t.Fatalf("failed record invalidations = %+v, want none", notices)
	}
}

func TestRunNowRejectsInvalidRunFactBeforeStarting(t *testing.T) {
	createdAt := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	store := &runNowStore{schedule: mustStoredSchedule(t, schedule.Snapshot{
		ID: "sch_1", Instructions: "review", CreatedAt: createdAt,
	})}
	runner := &cancelingScheduledRunStarter{cancel: func() {}, succeed: true}
	firing := mustFiring(t, FiringDependencies{
		Store: store, RunStarter: runner,
	})
	firing.now = func() time.Time { return createdAt.Add(-time.Millisecond) }

	if _, err := firing.RunNow(t.Context(), "sch_1"); err == nil {
		t.Fatal("RunNow accepted a run fact before Schedule creation")
	}
	if len(runner.startedScheduleIDs) != 0 {
		t.Fatalf("started schedules = %v, want none before valid run fact", runner.startedScheduleIDs)
	}
}

type cwdResolverFunc func(string) (string, error)

func (c cwdResolverFunc) ResolveExistingDir(path string) (string, error) {
	return c(path)
}

func TestCreateOwnsScheduleAdmission(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	store := &runNowStore{}
	c := mustCoordinator(t, Dependencies{
		Store:         store,
		Models:        allowModels{},
		NewScheduleID: fixedIdentity("sch_created"),
		Paths: cwdResolverFunc(func(path string) (string, error) {
			if path != "workspace" {
				t.Fatalf("ResolveExistingDir(%q), want workspace", path)
			}
			return "/canonical/workspace", nil
		}),
	})
	c.now = func() time.Time { return now }

	created, err := c.Create(t.Context(), CreateCommand{
		Instructions: "review",
		CWD:          "workspace",
		Cron:         "0 13 * * *",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.CWD() != "/canonical/workspace" || !created.Enabled() {
		t.Fatalf("created = %+v", created)
	}
	wantNext, err := schedule.NextRun("0 13 * * *", now)
	if err != nil {
		t.Fatalf("NextRun: %v", err)
	}
	if !created.NextRunAt().Equal(wantNext) {
		t.Fatalf("NextRunAt = %v, want %v", created.NextRunAt(), wantNext)
	}
}

func TestUpdateOwnsPatchAndPreservesSnapshotState(t *testing.T) {
	lastRun := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	store := &runNowStore{schedule: mustStoredSchedule(t, schedule.Snapshot{
		ID:           "sch_1",
		Revision:     3,
		Instructions: "before",
		CWD:          "/before",
		Cron:         "0 9 * * *",
		Enabled:      true,
		LastRunAt:    lastRun,
		CreatedAt:    createdAt,
		NextRunAt:    lastRun.Add(time.Hour),
	})}
	c := mustCoordinator(t, Dependencies{
		Store:  store,
		Models: allowModels{},
		Paths: cwdResolverFunc(func(string) (string, error) {
			return "/canonical/after", nil
		}),
	})
	cwd, instructions, enabled := "after", "after", false

	updated, err := c.Update(t.Context(), UpdateCommand{
		ID: "sch_1", ExpectedRevision: 3,
		Patch: schedule.Patch{
			Instructions: &instructions,
			CWD:          &cwd,
			Enabled:      &enabled,
		},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ID() != "sch_1" || updated.Instructions() != "after" || updated.CWD() != "/canonical/after" {
		t.Fatalf("updated = %+v", updated)
	}
	if !updated.LastRunAt().Equal(lastRun) || !updated.CreatedAt().Equal(createdAt) || !updated.NextRunAt().IsZero() {
		t.Fatalf("updated durable state = %+v", updated)
	}
}

func TestUpdateRequiresAnExplicitRevision(t *testing.T) {
	c := mustCoordinator(t, Dependencies{Store: &runNowStore{schedule: mustStoredSchedule(t, schedule.Snapshot{ID: "sch_1", Instructions: "review"})}, Models: allowModels{}})
	_, err := c.Update(t.Context(), UpdateCommand{ID: "sch_1"})
	if !errors.Is(err, schedule.ErrRevisionRequired) {
		t.Fatalf("Update error = %v, want ErrRevisionRequired", err)
	}
}

func TestCreateValidatesBeforeResolvingCWD(t *testing.T) {
	resolved := false
	c := mustCoordinator(t, Dependencies{
		Store:         &runNowStore{},
		Models:        allowModels{},
		NewScheduleID: fixedIdentity("sch_created"),
		Paths: cwdResolverFunc(func(string) (string, error) {
			resolved = true
			return "", errors.New("unexpected resolution")
		}),
	})
	_, err := c.Create(t.Context(), CreateCommand{CWD: "missing", Cron: "@daily", Enabled: true})
	if !errors.Is(err, schedule.ErrInstructionsRequired) {
		t.Fatalf("Create error = %v, want ErrInstructionsRequired", err)
	}
	if resolved {
		t.Fatal("cwd was resolved before schedule validation")
	}
}

type runNowStore struct {
	schedule     schedule.Schedule
	created      schedule.Schedule
	updated      schedule.Schedule
	recordedID   string
	recordedAt   time.Time
	recordCtxErr error
	recordErr    error
}

func (r *runNowStore) ListPage(ctx context.Context, _ time.Time, _ string, _ int) ([]schedule.Schedule, error) {
	return nil, nil
}
func (r *runNowStore) Get(context.Context, string) (schedule.Schedule, error) {
	return r.schedule, nil
}
func (r *runNowStore) Insert(_ context.Context, scheduled schedule.Schedule) error {
	r.created = scheduled
	return nil
}
func (r *runNowStore) Update(_ context.Context, scheduled schedule.Schedule, _ uint64) (schedule.Schedule, error) {
	r.updated = scheduled
	return scheduled, nil
}
func (r *runNowStore) Delete(context.Context, string) (bool, error) { return false, nil }
func (r *runNowStore) Due(context.Context, time.Time, int) ([]schedule.Schedule, error) {
	return nil, nil
}
func (r *runNowStore) Claim(context.Context, schedule.Claim) (bool, error) { return false, nil }
func (r *runNowStore) Pending(context.Context, int) ([]schedule.Occurrence, error) {
	return nil, nil
}
func (r *runNowStore) RecordRun(ctx context.Context, record schedule.RunRecord) error {
	r.recordedID, r.recordedAt, r.recordCtxErr = record.ScheduleID(), record.RanAt(), ctx.Err()
	return r.recordErr
}

// TestRunWorkerNoOpWithoutScheduling ensures a disabled schedule capability
// returns at once rather than entering a scan loop.
func TestRunWorkerNoOpWithoutWorker(t *testing.T) {
	firing := DisabledFiring()
	done := make(chan struct{})
	go func() {
		firing.RunWorker(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunWorker blocked without a worker store")
	}
}

// pagedStore seeks the way the store does: newest created first, id last so the
// order is total, and a zero anchor is the first page rather than a position before
// every row.
type pagedStore struct {
	*runNowStore
	rows []schedule.Schedule

	afterID string
	limit   int
}

func (p *pagedStore) ListPage(_ context.Context, afterCreatedAt time.Time, afterID string, limit int) ([]schedule.Schedule, error) {
	p.afterID, p.limit = afterID, limit
	var out []schedule.Schedule
	for _, row := range p.rows {
		if !afterCreatedAt.IsZero() || afterID != "" {
			if row.CreatedAt().After(afterCreatedAt) || (row.CreatedAt().Equal(afterCreatedAt) && row.ID() <= afterID) {
				continue
			}
		}
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, row)
	}
	return out, nil
}

func scheduleRows(t testing.TB, ids ...string) []schedule.Schedule {
	t.Helper()
	out := make([]schedule.Schedule, 0, len(ids))
	for i, id := range ids {
		out = append(out, mustStoredSchedule(t, schedule.Snapshot{
			ID: id, Instructions: "review", CreatedAt: time.Unix(0, int64(len(ids)-i)).UTC(),
		}))
	}
	return out
}

// TestListPagePagesNewestFirstAndRefusesAForeignCursor covers the schedules query
// properties: the order is fixed (newest created first, id breaking ties), the
// next page seeks strictly past the previous one, and a cursor from another query is
// refused rather than quietly restarting — a schedule shown twice reads as a second
// schedule that fires on the same cron.
func TestListPagePagesNewestFirstAndRefusesAForeignCursor(t *testing.T) {
	store := &pagedStore{runNowStore: &runNowStore{}, rows: scheduleRows(t, "sch_1", "sch_2", "sch_3")}
	c := mustCoordinator(t, Dependencies{Store: store, Models: allowModels{}})
	ctx := t.Context()

	first, err := c.ListPage(ctx, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if store.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", store.limit)
	}
	if len(first.Rows) != 2 || first.Rows[0].ID() != "sch_1" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two schedules and a cursor", first.Rows)
	}

	second, err := c.ListPage(ctx, first.NextCursor, explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if store.afterID != "sch_2" {
		t.Fatalf("second page sought past %q, want the first page's last row", store.afterID)
	}
	if len(second.Rows) != 1 || second.Rows[0].ID() != "sch_3" || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want the tail and no cursor", second.Rows)
	}

	foreign, err := pagination.Encode("sessions", nil, []string{"0", "sch_1"})
	if err != nil {
		t.Fatalf("encode foreign cursor: %v", err)
	}
	if _, err := c.ListPage(ctx, foreign, explicitPageLimit(t, 2)); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("cursor from another query err = %v, want ErrInvalidCursor", err)
	}
	if _, err := c.ListPage(ctx, first.NextCursor+"x", explicitPageLimit(t, 2)); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("damaged cursor err = %v, want ErrInvalidCursor", err)
	}
}

func mustStoredSchedule(t testing.TB, snapshot schedule.Snapshot) schedule.Schedule {
	t.Helper()
	if snapshot.Cron == "" {
		snapshot.Cron = "@daily"
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Unix(1, 0).UTC()
	}
	if snapshot.Revision == 0 {
		snapshot.Revision = 1
	}
	if !snapshot.Enabled && snapshot.NextRunAt.IsZero() {
		// Disabled is a valid explicit fixture state.
	} else if snapshot.NextRunAt.IsZero() {
		snapshot.Enabled = true
		snapshot.NextRunAt = snapshot.CreatedAt.Add(time.Hour)
	}
	value, err := schedule.Restore(snapshot)
	if err != nil {
		t.Fatalf("restore schedule fixture: %v", err)
	}
	return value
}
