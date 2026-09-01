package sqlite_test

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func newGoalStore(t *testing.T) (*sqlite.GoalStore, *sqlite.SessionStore) {
	t.Helper()
	goals, sessions, _ := newGoalRunStores(t)
	return goals, sessions
}

func newGoalRunStores(t *testing.T) (*sqlite.GoalStore, *sqlite.SessionStore, *sqlite.RunStore) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewGoalStore(db), sqlite.NewSessionStore(db), sqlite.NewRunStore(db)
}

func readGoal(ctx context.Context, store *sqlite.GoalStore, sessionID string) (goal.Goal, bool, error) {
	current, err := store.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, false, err
	}
	value, exists := current.Goal()
	return value, exists, nil
}

func unwrittenVersion(t *testing.T, sessionID string) goal.Version {
	t.Helper()
	current, err := goal.Unwritten(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return current.Version()
}

func goalRunCost(t *testing.T, usd float64) accounting.Cost {
	t.Helper()
	cost, err := accounting.NewCost(usd)
	if err != nil {
		t.Fatalf("NewCost(%g): %v", usd, err)
	}
	return cost
}

func persistTerminalGoalRun(t *testing.T, store *sqlite.RunStore, record goal.RunRecord) {
	t.Helper()
	outcome := record.Outcome
	value := testsupport.MustRestoreRun(run.Snapshot{
		SessionID: record.SessionID, ID: record.RunID,
		GoalIncarnationID: record.IncarnationID,
		Outcome:           &outcome,
		Metrics: testsupport.MustRunMetrics(testsupport.RunMetricsInput{
			Steps: record.Steps,
			Usage: &accounting.Usage{Total: accounting.Totals{CostUSD: record.Cost.OptionalUSD()}},
		}),
		CreatedAt: record.CompletedAt.Add(-time.Second), FinishedAt: record.CompletedAt,
		UpdatedAt: record.CompletedAt, MessageMark: 0,
	})
	if err := store.Restore(t.Context(), value); err != nil {
		t.Fatalf("persist terminal Goal Run: %v", err)
	}
}

func TestGoalStoreRecordRunIsIdempotentAndBlocksAtBudget(t *testing.T) {
	store, sessions, runs := newGoalRunStores(t)
	const sessionID = "ses_goal_run"
	seedSession(t, sessions, sessionID)
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	g, err := goal.New(sessionID, "finish", testReasoningSelection(t, "provider", "model", ""), limitedBudget(t, goal.BudgetLimits{MaxRuns: intLimit(1)}), run.Capabilities{}, "lease_goal_run", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, applied, saveErr := store.Save(t.Context(), g, unwrittenVersion(t, sessionID)); saveErr != nil || !applied {
		t.Fatalf("Save = (%v, %v), want true, nil", applied, saveErr)
	}
	record := goal.RunRecord{
		SessionID: sessionID, IncarnationID: g.IncarnationID(), RunID: "run_goal_run",
		Outcome: run.OutcomeCompleted, Cost: goalRunCost(t, 0.25), Steps: 3, CompletedAt: now.Add(time.Minute),
	}
	if recordRunErr := store.RecordRun(t.Context(), record); !errors.Is(recordRunErr, goal.ErrRunIdentityConflict) {
		t.Fatalf("ownerless RecordRun = %v, want ErrRunIdentityConflict", recordRunErr)
	}
	persistTerminalGoalRun(t, runs, record)
	if recordRunErr := store.RecordRun(t.Context(), record); recordRunErr != nil {
		t.Fatalf("RecordRun: %v", recordRunErr)
	}
	if recordRunErr := store.RecordRun(t.Context(), record); recordRunErr != nil {
		t.Fatalf("repeat RecordRun: %v", recordRunErr)
	}
	conflict := record
	conflict.IncarnationID = "another_lease"
	if recordRunErr := store.RecordRun(t.Context(), conflict); !errors.Is(recordRunErr, goal.ErrRunIdentityConflict) {
		t.Fatalf("conflicting RecordRun = %v, want ErrRunIdentityConflict", recordRunErr)
	}
	got, found, err := readGoal(t.Context(), store, sessionID)
	if err != nil || !found {
		t.Fatalf("Get = (%v, %v), want found", found, err)
	}
	if got.Used() != (goal.Usage{Runs: 1, Cost: goalRunCost(t, 0.25), Steps: 3}) || got.Status() != goal.StatusBlocked || got.Reason().Code() != goal.ReasonRunBudgetReached {
		t.Fatalf("goal after idempotent RecordRun = %+v", got)
	}
	if err := runs.Delete(t.Context(), record.SessionID, record.RunID); err != nil {
		t.Fatalf("delete terminal Run: %v", err)
	}
	if recordRunErr := store.RecordRun(t.Context(), record); !errors.Is(recordRunErr, goal.ErrRunIdentityConflict) {
		t.Fatalf("RecordRun after owner deletion = %v, want ErrRunIdentityConflict", recordRunErr)
	}
}

func TestGoalStorePreservesUnavailableRunPricing(t *testing.T) {
	store, sessions, runs := newGoalRunStores(t)
	const sessionID = "ses_unpriced_goal_run"
	seedSession(t, sessions, sessionID)
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	g, err := goal.New(
		sessionID,
		"finish safely",
		testReasoningSelection(t, "private", "served-alias", ""),
		limitedBudget(t, goal.BudgetLimits{MaxCostUSD: costLimit(1)}),
		run.Capabilities{},
		"lease_unpriced_goal_run",
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, applied, saveErr := store.Save(t.Context(), g, unwrittenVersion(t, sessionID)); saveErr != nil || !applied {
		t.Fatalf("Save = (%v, %v), want true, nil", applied, saveErr)
	}
	record := goal.RunRecord{
		SessionID: sessionID, IncarnationID: g.IncarnationID(), RunID: "run_unpriced_goal_run",
		Outcome: run.OutcomeCompleted, Steps: 2, CompletedAt: now.Add(time.Minute),
	}
	persistTerminalGoalRun(t, runs, record)
	if err := store.RecordRun(t.Context(), record); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}
	if err := store.RecordRun(t.Context(), record); err != nil {
		t.Fatalf("repeat RecordRun: %v", err)
	}
	got, found, err := readGoal(t.Context(), store, sessionID)
	if err != nil || !found {
		t.Fatalf("Get = (%v, %v), want found", found, err)
	}
	if got.Status() != goal.StatusBlocked || got.Reason().Code() != goal.ReasonPricingUnavailable ||
		got.Used() != (goal.Usage{Runs: 1, Steps: 2}) {
		t.Fatalf("Goal after unpriced Run = %+v", got.Snapshot())
	}
}

func TestGoalSchemaUsesSemanticIncarnationColumns(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	columnsOf := func(table string) []string {
		t.Helper()
		rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
		if err != nil {
			t.Fatalf("table_info(%s): %v", table, err)
		}
		defer func() { _ = rows.Close() }()
		var columns []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatalf("scan table_info(%s): %v", table, err)
			}
			columns = append(columns, name)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("table_info(%s): %v", table, err)
		}
		return columns
	}

	for _, test := range []struct {
		table    string
		current  string
		obsolete string
	}{
		{table: "goals", current: "incarnation_id", obsolete: "lease_id"},
		{table: "goal_runs", current: "incarnation_id", obsolete: "lease_id"},
		{table: "runs", current: "goal_incarnation_id", obsolete: "goal_lease_id"},
		{table: "interrupts", current: "goal_incarnation_id", obsolete: "goal_lease_id"},
	} {
		columns := columnsOf(test.table)
		if !slices.Contains(columns, test.current) {
			t.Errorf("%s columns = %v, want %s", test.table, columns, test.current)
		}
		if slices.Contains(columns, test.obsolete) {
			t.Errorf("%s columns retain obsolete %s: %v", test.table, test.obsolete, columns)
		}
	}

	goalColumns := columnsOf("goals")
	if !slices.Contains(goalColumns, "reason_code") {
		t.Errorf("goals columns = %v, want reason_code", goalColumns)
	}
	if slices.Contains(goalColumns, "reason_cause") {
		t.Errorf("goals columns retain obsolete reason_cause: %v", goalColumns)
	}
}

func seedSession(t *testing.T, store *sqlite.SessionStore, id string) {
	t.Helper()
	value := testsupport.MustRestoreSession(session.Snapshot{ID: id, Workspace: testsupport.MustWorkspace("/work")})
	if err := store.Insert(t.Context(), value); err != nil {
		t.Fatalf("seed session %q: %v", id, err)
	}
}

func TestGoalStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	store, sessions := newGoalStore(t)
	const sess = "sess-goal"
	seedSession(t, sessions, sess)

	if _, ok, err := readGoal(ctx, store, sess); err != nil || ok {
		t.Fatalf("Get(unknown) = (%v, %v), want (false, nil)", ok, err)
	}

	now := time.Unix(1_700_000_000, 0).UTC()
	wantCapabilities := run.Capabilities{
		ChildRuns:      true,
		InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Question},
	}
	selection := testReasoningSelection(t, "anthropic", "claude", "high")
	g, err := goal.New(sess, "ship the feature", selection, limitedBudget(t, goal.BudgetLimits{MaxRuns: intLimit(5), MaxCostUSD: costLimit(2.5)}), wantCapabilities, "lease-round-trip", now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot := g.Snapshot()
	snapshot.Used = goal.Usage{Runs: 1, Cost: goalRunCost(t, 0.4), Steps: 3}
	g, err = goal.Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, applied, saveErr := store.Save(ctx, g, unwrittenVersion(t, sess)); saveErr != nil || !applied {
		t.Fatalf("Save: applied=%v err=%v", applied, saveErr)
	}

	got, ok, err := readGoal(ctx, store, sess)
	if err != nil || !ok {
		t.Fatalf("Get = (%v, %v), want (true, nil)", ok, err)
	}
	budget, used, gotSelection := got.Budget(), got.Used(), got.ModelSelection()
	maxRuns, runsLimited := budget.MaxRuns()
	maxCostUSD, costLimited := budget.MaxCostUSD()
	if got.Objective() != "ship the feature" || got.Status() != goal.StatusActive ||
		!runsLimited || maxRuns != 5 || !costLimited || maxCostUSD != 2.5 ||
		used.Runs != 1 || !used.Cost.Equal(goalRunCost(t, 0.4)) || used.Steps != 3 ||
		gotSelection.Provider() != "anthropic" || gotSelection.Model() != "claude" ||
		gotSelection.ReasoningEffort() != "high" ||
		!got.Capabilities().Equal(wantCapabilities) {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if !got.CreatedAt().Equal(now) {
		t.Fatalf("created_at = %v, want %v", got.CreatedAt(), now)
	}
}

func testReasoningSelection(t testing.TB, provider, model, effort string) modelref.Selection {
	t.Helper()
	selection, err := modelref.NewWithReasoningEffort(provider, model, effort)
	if err != nil {
		t.Fatalf("modelref.NewWithReasoningEffort(%q, %q, %q): %v", provider, model, effort, err)
	}
	return selection
}

func limitedBudget(t testing.TB, limits goal.BudgetLimits) goal.Budget {
	t.Helper()
	budget, err := goal.NewBudget(limits)
	if err != nil {
		t.Fatal(err)
	}
	return budget
}

func intLimit(value int) *int { return &value }

func costLimit(value float64) *float64 { return &value }

func TestGoalStore_ListAndClear(t *testing.T) {
	ctx := context.Background()
	store, sessions := newGoalStore(t)
	now := time.Unix(1_700_000_000, 0).UTC()

	for _, s := range []string{"a", "b"} {
		seedSession(t, sessions, s)
		g, _ := goal.New(s, "obj-"+s, testReasoningSelection(t, "provider", "model", ""), goal.UnlimitedBudget(), run.Capabilities{}, "lease-"+s, now)
		if _, applied, err := store.Save(ctx, g, unwrittenVersion(t, s)); err != nil || !applied {
			t.Fatalf("Save(%s): applied=%v err=%v", s, applied, err)
		}
	}
	all, err := store.List(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("List = (%d, %v), want 2", len(all), err)
	}

	if err := store.Clear(ctx, "a"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, ok, _ := readGoal(ctx, store, "a"); ok {
		t.Fatal("cleared goal still present")
	}
	if _, ok, _ := readGoal(ctx, store, "b"); !ok {
		t.Fatal("Clear removed the wrong session")
	}
	// Clearing a missing goal is not an error.
	if err := store.Clear(ctx, "missing"); err != nil {
		t.Fatalf("Clear(missing): %v", err)
	}
}

// TestGoalStore_CompareAndSwap covers the keystone CAS: insert-if-absent on
// explicit unwritten state, update-if-version-matches otherwise, and reject a stale writer
// (including ClearIf) so a superseded loop can neither clobber a newer goal nor
// resurrect a cleared one.
func TestGoalStore_CompareAndSwap(t *testing.T) {
	ctx := context.Background()
	store, sessions := newGoalStore(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	const sess = "s"
	seedSession(t, sessions, sess)

	initial, err := goal.New(sess, "obj", testReasoningSelection(t, "provider", "model", ""), goal.UnlimitedBudget(), run.Capabilities{}, "lease-one", now)
	if err != nil {
		t.Fatal(err)
	}
	// The unwritten version inserts when absent, then refuses a second insert.
	if _, applied, err := store.Save(ctx, initial, unwrittenVersion(t, sess)); err != nil || !applied {
		t.Fatalf("insert: applied=%v err=%v", applied, err)
	}
	if _, applied, _ := store.Save(ctx, initial, unwrittenVersion(t, sess)); applied {
		t.Fatal("unwritten version must not overwrite an existing goal")
	}

	// A stale writer (unwritten expectation, wrong incarnation, or wrong revision) is rejected — no
	// clobber, no resurrection.
	staleVersionSnapshot := initial.Snapshot()
	staleVersionSnapshot.Revision = 99
	staleVersionOwner, restoreErr := goal.Restore(staleVersionSnapshot)
	if restoreErr != nil {
		t.Fatal(restoreErr)
	}
	replacement, err := goal.New(sess, "replacement", testReasoningSelection(t, "provider", "model", ""), goal.UnlimitedBudget(), run.Capabilities{}, "lease-two", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, applied, _ := store.Save(ctx, replacement, staleVersionOwner.Version()); applied {
		t.Fatal("mismatched revision must not apply")
	}
	// A lifecycle transition preserves the objective incarnation and arrives
	// with its domain-decided next revision.
	paused, err := initial.Pause(goal.ReasonStoppedByUser, "", now)
	if err != nil {
		t.Fatal(err)
	}
	var applied bool
	if paused, applied, err = store.Save(ctx, paused, initial.Version()); err != nil || !applied {
		t.Fatalf("cas update: applied=%v err=%v", applied, err)
	}
	got, _, _ := readGoal(ctx, store, sess)
	if got.Version() != paused.Version() || got.Status() != goal.StatusPaused {
		t.Fatalf("after cas: version=%+v status=%q, want %+v/paused", got.Version(), got.Status(), paused.Version())
	}

	// A same-incarnation mutation advances revision and rejects the prior revision.
	resumed, err := paused.Resume(now)
	if err != nil {
		t.Fatal(err)
	}
	if resumed, applied, err = store.Save(ctx, resumed, paused.Version()); err != nil || !applied {
		t.Fatalf("same-incarnation update: applied=%v err=%v", applied, err)
	}
	if applied, _ := store.ClearIf(ctx, sess, paused.Version()); applied {
		t.Fatal("ClearIf must not delete on a stale revision")
	}
	if applied, err := store.ClearIf(ctx, sess, resumed.Version()); err != nil || !applied {
		t.Fatalf("ClearIf(match): applied=%v err=%v", applied, err)
	}
	if _, ok, _ := readGoal(ctx, store, sess); ok {
		t.Fatal("goal should be gone after a matching ClearIf")
	}
}

func TestGoalStoreReplacesExistingGoalWithoutCallerRevision(t *testing.T) {
	store, sessions := newGoalStore(t)
	const sessionID = "s"
	seedSession(t, sessions, sessionID)
	now := time.Unix(1_700_000_000, 0).UTC()

	first, _ := goal.New(sessionID, "first", testReasoningSelection(t, "provider", "model", ""), goal.UnlimitedBudget(), run.Capabilities{}, "lease-first", now)
	first, applied, err := store.Save(t.Context(), first, unwrittenVersion(t, sessionID))
	if err != nil || !applied {
		t.Fatalf("insert first goal: applied=%v err=%v", applied, err)
	}
	firstVersion := first.Version()
	first, err = first.Pause(goal.ReasonStoppedByUser, "", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	first, applied, err = store.Save(t.Context(), first, firstVersion)
	if err != nil || !applied {
		t.Fatalf("stop first goal: applied=%v err=%v", applied, err)
	}

	fresh, _ := goal.New(sessionID, "second", testReasoningSelection(t, "provider", "model", ""), goal.UnlimitedBudget(), run.Capabilities{}, "lease-second", now.Add(2*time.Second))
	fresh, applied, err = store.Save(t.Context(), fresh, first.Version())
	if err != nil || !applied {
		t.Fatalf("replace goal: applied=%v err=%v", applied, err)
	}
	if fresh.Revision() != 1 || fresh.Objective() != "second" || fresh.IncarnationID() != "lease-second" {
		t.Fatalf("replacement = %+v, previous = %+v", fresh, first)
	}
}

func TestGoalStore_ClearThenRecreateRejectsStaleIncarnation(t *testing.T) {
	store, sessions := newGoalStore(t)
	const sessionID = "s"
	seedSession(t, sessions, sessionID)
	now := time.Unix(1_700_000_000, 0).UTC()

	stale, _ := goal.New(sessionID, "old", testReasoningSelection(t, "provider", "model", ""), goal.UnlimitedBudget(), run.Capabilities{}, "lease-old", now)
	staleVersion := stale.Version()
	if _, applied, err := store.Save(t.Context(), stale, unwrittenVersion(t, sessionID)); err != nil || !applied {
		t.Fatalf("seed stale goal: applied=%v err=%v", applied, err)
	}
	if err := store.Clear(t.Context(), sessionID); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	fresh, _ := goal.New(sessionID, "new", testReasoningSelection(t, "provider", "model", ""), goal.UnlimitedBudget(), run.Capabilities{}, "lease-fresh", now)
	if _, applied, err := store.Save(t.Context(), fresh, unwrittenVersion(t, sessionID)); err != nil || !applied {
		t.Fatalf("seed fresh goal: applied=%v err=%v", applied, err)
	}

	stale, _ = stale.Pause(goal.ReasonRunNotCompleted, "error", now)
	if _, applied, err := store.Save(t.Context(), stale, staleVersion); err != nil || applied {
		t.Fatalf("stale Save: applied=%v err=%v, want false/nil", applied, err)
	}
	if applied, err := store.ClearIf(t.Context(), sessionID, staleVersion); err != nil || applied {
		t.Fatalf("stale ClearIf: applied=%v err=%v, want false/nil", applied, err)
	}
	got, ok, err := readGoal(t.Context(), store, sessionID)
	if err != nil || !ok || got.Objective() != "new" || got.IncarnationID() != "lease-fresh" {
		t.Fatalf("fresh goal was changed: goal=%+v present=%v err=%v", got, ok, err)
	}
}

// TestGoalStoreRejectsMissingSession is the lifecycle boundary's evidence for
// goal_never_outlives_its_session: the CAS that opens a goal cannot open one for a
// session that is not there, so no lifecycle transition can resurrect a goal whose
// session has already gone.
func TestGoalStoreRejectsMissingSession(t *testing.T) {
	store, _ := newGoalStore(t)
	g, _ := goal.New("missing", "obj", testReasoningSelection(t, "provider", "model", ""), goal.UnlimitedBudget(), run.Capabilities{}, "lease-missing", time.Unix(0, 0))
	if _, applied, err := store.Save(t.Context(), g, unwrittenVersion(t, "missing")); err == nil || applied {
		t.Fatalf("Save(missing session) = applied=%v err=%v, want false/non-nil", applied, err)
	}
}

func TestGoalStoreCascadesWithSessionDeletion(t *testing.T) {
	store, sessions, runs := newGoalRunStores(t)
	const sessionID = "s"
	seedSession(t, sessions, sessionID)
	g, _ := goal.New(sessionID, "obj", testReasoningSelection(t, "provider", "model", ""), goal.UnlimitedBudget(), run.Capabilities{}, "lease", time.Unix(0, 0))
	if _, applied, err := store.Save(t.Context(), g, unwrittenVersion(t, sessionID)); err != nil || !applied {
		t.Fatalf("seed goal: applied=%v err=%v", applied, err)
	}
	record := goal.RunRecord{
		SessionID: sessionID, IncarnationID: g.IncarnationID(), RunID: "run-reusable-after-delete",
		Outcome: run.OutcomeCompleted, CompletedAt: time.Unix(1, 0),
	}
	persistTerminalGoalRun(t, runs, record)
	if err := store.RecordRun(t.Context(), record); err != nil {
		t.Fatalf("record old Goal Run: %v", err)
	}

	if err := sessions.Delete(t.Context(), sessionID); err != nil {
		t.Fatalf("Delete(session): %v", err)
	}
	if _, ok, err := readGoal(t.Context(), store, sessionID); err != nil || ok {
		t.Fatalf("goal after session delete = present=%v err=%v, want false/nil", ok, err)
	}
	if err := runs.DeleteForSession(t.Context(), sessionID); err != nil {
		t.Fatalf("delete old Session Runs: %v", err)
	}

	// Reusing the same ids proves the old idempotency ledger row was owned by and
	// cascaded with the deleted Session.
	seedSession(t, sessions, sessionID)
	recreated, _ := goal.New(sessionID, "new", testReasoningSelection(t, "provider", "model", ""), goal.UnlimitedBudget(), run.Capabilities{}, "lease-new", time.Unix(2, 0))
	if _, applied, err := store.Save(t.Context(), recreated, unwrittenVersion(t, sessionID)); err != nil || !applied {
		t.Fatalf("seed recreated goal: applied=%v err=%v", applied, err)
	}
	record.IncarnationID = recreated.IncarnationID()
	record.CompletedAt = time.Unix(3, 0)
	persistTerminalGoalRun(t, runs, record)
	if err := store.RecordRun(t.Context(), record); err != nil {
		t.Fatalf("reuse terminal identity after session deletion: %v", err)
	}
	got, ok, err := readGoal(t.Context(), store, sessionID)
	if err != nil || !ok || got.Used().Runs != 1 {
		t.Fatalf("recreated goal accounting = %+v, present=%v err=%v", got.Used(), ok, err)
	}
}

func TestGoalStoreExecutesDomainDecidedRevision(t *testing.T) {
	store, sessions := newGoalStore(t)
	const sessionID = "s"
	seedSession(t, sessions, sessionID)
	g, _ := goal.New(sessionID, "obj", testReasoningSelection(t, "provider", "model", ""), goal.UnlimitedBudget(), run.Capabilities{}, "lease", time.Unix(0, 0))
	saved, applied, err := store.Save(t.Context(), g, unwrittenVersion(t, sessionID))
	if err != nil || !applied || saved.Revision() != 1 {
		t.Fatalf("insert = revision %d, applied=%v err=%v, want 1/true/nil", saved.Revision(), applied, err)
	}

	invalidSnapshot := saved.Snapshot()
	invalidSnapshot.Revision = 99
	invalidAdvance, restoreErr := goal.Restore(invalidSnapshot)
	if restoreErr != nil {
		t.Fatal(restoreErr)
	}
	if _, applied, err := store.Save(t.Context(), invalidAdvance, saved.Version()); err == nil || applied {
		t.Fatalf("Save(non-advancing replacement) = applied=%v err=%v, want false/non-nil", applied, err)
	}

	updated, err := saved.Pause(goal.ReasonStoppedByUser, "", time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	updated, applied, err = store.Save(t.Context(), updated, saved.Version())
	if err != nil || !applied || updated.Revision() != 2 {
		t.Fatalf("update = revision %d, applied=%v err=%v, want 2/true/nil", updated.Revision(), applied, err)
	}

	exhaustedSnapshot := updated.Snapshot()
	exhaustedSnapshot.Revision = math.MaxInt64
	exhausted, restoreErr := goal.Restore(exhaustedSnapshot)
	if restoreErr != nil {
		t.Fatal(restoreErr)
	}
	if _, applied, err := store.Save(t.Context(), exhausted, exhausted.Version()); err == nil || applied {
		t.Fatalf("Save(exhausted revision) = applied=%v err=%v, want false/non-nil", applied, err)
	}
}
