package schedules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
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

func TestScheduleConstructorsRejectPartialAndTypedNilDependencies(t *testing.T) {
	store := &runNowStore{}
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

	created, err := c.Create(t.Context(), schedule.Draft{
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

func TestUpdatePreservesAcceptedScheduleAfterCallerReuse(t *testing.T) {
	lastRun := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	store := &runNowStore{schedule: mustStoredSchedule(t, schedule.Snapshot{
		ID:           "sch_1",
		Revision:     3,
		Instructions: "before",
		CWD:          "/before",
		Cron:         "0 9 * * *",
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
	title, instructions, cwd := "Review", "after", "after"
	provider, model, effort, cron := "provider", "model", "high", "0 15 * * *"
	enabled := false

	updated, err := c.Update(t.Context(), UpdateCommand{
		ID: "sch_1", ExpectedRevision: 3,
		Patch: Patch{
			Title:        &title,
			Instructions: &instructions,
			CWD:          &cwd,
			ModelSelection: modelref.Patch{
				Provider: &provider, Model: &model, ReasoningEffort: &effort,
			},
			Cron:    &cron,
			Enabled: &enabled,
		},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ID() != "sch_1" || updated.Title() != title || updated.Instructions() != instructions ||
		updated.CWD() != "/canonical/after" || updated.Cron() != cron || updated.Enabled() || updated.Revision() != 4 {
		t.Fatalf("updated = %+v", updated)
	}
	if selection := updated.ModelSelection(); selection.Provider() != provider || selection.Model() != model ||
		selection.ReasoningEffort() != effort {
		t.Fatalf("updated model selection = %+v", selection)
	}
	if cwd != "after" {
		t.Fatalf("workspace resolution mutated the caller's patch to %q", cwd)
	}
	if !updated.LastRunAt().Equal(lastRun) || !updated.CreatedAt().Equal(createdAt) || !updated.NextRunAt().IsZero() {
		t.Fatalf("updated durable state = %+v", updated)
	}
	accepted := updated.Snapshot()
	title, instructions, cwd = "reused", "reused", "/reused"
	provider, model, effort, cron = "other-provider", "other-model", "low", "@hourly"
	enabled = true
	if updated.Snapshot() != accepted || store.updated.Snapshot() != accepted {
		t.Fatal("caller reuse changed the returned or stored Schedule")
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
	_, err := c.Create(t.Context(), schedule.Draft{CWD: "missing", Cron: "@daily", Enabled: true})
	if !errors.Is(err, schedule.ErrInstructionsRequired) {
		t.Fatalf("Create error = %v, want ErrInstructionsRequired", err)
	}
	if resolved {
		t.Fatal("cwd was resolved before schedule validation")
	}
}

type runNowStore struct {
	schedule schedule.Schedule
	created  schedule.Schedule
	updated  schedule.Schedule
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
func (r *runNowStore) Update(_ context.Context, replacement schedule.Replacement) error {
	r.updated = replacement.State()
	return nil
}
func (r *runNowStore) Delete(context.Context, string) (bool, error) { return false, nil }
func (r *runNowStore) Due(context.Context, time.Time, int) ([]schedule.Schedule, error) {
	return nil, nil
}
func (r *runNowStore) Claim(context.Context, schedule.Claim) (bool, error) { return false, nil }
func (r *runNowStore) Pending(context.Context, time.Time, string, int) ([]schedule.Occurrence, error) {
	return nil, nil
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
			ID: id, Instructions: "review", CreatedAt: time.UnixMilli(int64(len(ids) - i)).UTC(),
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
	value, err := schedule.Restore(snapshot)
	if err != nil {
		t.Fatalf("restore schedule fixture: %v", err)
	}
	return value
}
