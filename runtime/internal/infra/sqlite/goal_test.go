package sqlite_test

import (
	"context"
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

func goalReplacement(t *testing.T, state goal.Goal, expected goal.Version) goal.Replacement {
	t.Helper()
	replacement, err := goal.NewReplacement(expected, state)
	if err != nil {
		t.Fatalf("prepare Goal replacement: %v", err)
	}
	return replacement
}

func goalRunCost(t *testing.T, usd float64) accounting.Cost {
	t.Helper()
	cost, err := accounting.NewCost(usd)
	if err != nil {
		t.Fatalf("NewCost(%g): %v", usd, err)
	}
	return cost
}

func persistTerminalGoalRun(
	t *testing.T,
	store *sqlite.RunStore,
	sessionID, incarnationID, runID string,
	outcome run.Outcome,
	costUSD *float64,
	steps int,
	completedAt time.Time,
) run.Run {
	t.Helper()
	value := testsupport.MustRestoreRun(run.Snapshot{
		SessionID: sessionID, ID: runID,
		GoalIncarnationID: incarnationID,
		Outcome:           &outcome,
		Metrics: testsupport.MustRunMetrics(testsupport.RunMetricsInput{
			Steps: steps,
			Usage: &accounting.Usage{Total: accounting.Totals{CostUSD: costUSD}},
		}),
		CreatedAt: completedAt.Add(-time.Second), FinishedAt: completedAt,
		UpdatedAt: completedAt, MessageMark: run.MessageMarkAt(0),
	})
	if err := store.Restore(t.Context(), value); err != nil {
		t.Fatalf("persist terminal Goal Run: %v", err)
	}
	return value
}

// TestGoalStoreFoldsUsageFromTheIncarnationsRuns proves Goal usage has no copy
// of its own: it is the incarnation's terminal Runs, so it follows them.
func TestGoalStoreFoldsUsageFromTheIncarnationsRuns(t *testing.T) {
	store, sessions, runs := newGoalRunStores(t)
	const sessionID = "ses_goal_run"
	seedSession(t, sessions, sessionID)
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	g, err := goal.New(sessionID, "finish", testReasoningSelection(t, "provider", "model", ""), run.Capabilities{}, "lease_goal_run", now)
	if err != nil {
		t.Fatal(err)
	}
	if applied, saveErr := store.Save(t.Context(), goalReplacement(t, g, unwrittenVersion(t, sessionID))); saveErr != nil || !applied {
		t.Fatalf("Save = (%v, %v), want true, nil", applied, saveErr)
	}
	persistTerminalGoalRun(t, runs, sessionID, g.IncarnationID(), "run_goal_run", run.OutcomeCompleted, new(0.25), 3, now.Add(time.Minute))
	persistTerminalGoalRun(t, runs, sessionID, "another_lease", "run_other_goal", run.OutcomeCompleted, new(1.0), 7, now.Add(time.Minute))
	got, found, err := readGoal(t.Context(), store, sessionID)
	if err != nil || !found {
		t.Fatalf("Get = (%v, %v), want found", found, err)
	}
	if got.Used() != (goal.Usage{Runs: 1, Cost: goalRunCost(t, 0.25), Steps: 3}) || got.Status() != goal.StatusActive ||
		!got.Reason().IsNone() || got.Revision() != g.Revision() {
		t.Fatalf("goal after completed Run = %+v", got.Snapshot())
	}

	canceled := persistTerminalGoalRun(t, runs, sessionID, g.IncarnationID(), "run_canceled", run.OutcomeCanceled, nil, 2, now.Add(2*time.Minute))
	got, _, err = readGoal(t.Context(), store, sessionID)
	if _, priced := got.Used().Cost.USD(); err != nil ||
		got.Used().Runs != 2 || got.Used().Steps != 5 || priced {
		t.Fatalf("goal after canceled Run = %+v, %v", got.Snapshot(), err)
	}

	if err := runs.Delete(t.Context(), sessionID, canceled.ID()); err != nil {
		t.Fatalf("delete terminal Run: %v", err)
	}
	got, _, err = readGoal(t.Context(), store, sessionID)
	if err != nil || got.Used() != (goal.Usage{Runs: 1, Cost: goalRunCost(t, 0.25), Steps: 3}) {
		t.Fatalf("usage after Run deletion = %+v, %v", got.Used(), err)
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
		{table: "runs", current: "goal_incarnation_id", obsolete: "goal_lease_id"},
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
	g, err := goal.New(sess, "ship the feature", selection, wantCapabilities, "lease-round-trip", now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if applied, saveErr := store.Save(ctx, goalReplacement(t, g, unwrittenVersion(t, sess))); saveErr != nil || !applied {
		t.Fatalf("Save: applied=%v err=%v", applied, saveErr)
	}

	got, ok, err := readGoal(ctx, store, sess)
	if err != nil || !ok {
		t.Fatalf("Get = (%v, %v), want (true, nil)", ok, err)
	}
	used, gotSelection := got.Used(), got.ModelSelection()
	if got.Objective() != "ship the feature" || got.Status() != goal.StatusActive ||
		used != (goal.Usage{}) ||
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

func TestGoalStore_ListAndClear(t *testing.T) {
	ctx := context.Background()
	store, sessions := newGoalStore(t)
	now := time.Unix(1_700_000_000, 0).UTC()

	for _, s := range []string{"a", "b"} {
		seedSession(t, sessions, s)
		g, _ := goal.New(s, "obj-"+s, testReasoningSelection(t, "provider", "model", ""), run.Capabilities{}, "lease-"+s, now)
		if applied, err := store.Save(ctx, goalReplacement(t, g, unwrittenVersion(t, s))); err != nil || !applied {
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

	initial, err := goal.New(sess, "obj", testReasoningSelection(t, "provider", "model", ""), run.Capabilities{}, "lease-one", now)
	if err != nil {
		t.Fatal(err)
	}
	// The unwritten version inserts when absent, then refuses a second insert.
	if applied, err := store.Save(ctx, goalReplacement(t, initial, unwrittenVersion(t, sess))); err != nil || !applied {
		t.Fatalf("insert: applied=%v err=%v", applied, err)
	}
	if applied, _ := store.Save(ctx, goalReplacement(t, initial, unwrittenVersion(t, sess))); applied {
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
	replacement, err := goal.New(sess, "replacement", testReasoningSelection(t, "provider", "model", ""), run.Capabilities{}, "lease-two", now)
	if err != nil {
		t.Fatal(err)
	}
	if applied, _ := store.Save(ctx, goalReplacement(t, replacement, staleVersionOwner.Version())); applied {
		t.Fatal("mismatched revision must not apply")
	}
	// A lifecycle transition preserves the objective incarnation and arrives
	// with its domain-decided next revision.
	paused, err := initial.Pause(goal.ReasonStoppedByUser, "", now)
	if err != nil {
		t.Fatal(err)
	}
	var applied bool
	if applied, err = store.Save(ctx, goalReplacement(t, paused, initial.Version())); err != nil || !applied {
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
	if applied, err = store.Save(ctx, goalReplacement(t, resumed, paused.Version())); err != nil || !applied {
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

func TestGoalStoreReplacesIncarnationWithoutRewritingRevision(t *testing.T) {
	store, sessions := newGoalStore(t)
	const sessionID = "s"
	seedSession(t, sessions, sessionID)
	now := time.Unix(1_700_000_000, 0).UTC()

	first, _ := goal.New(sessionID, "first", testReasoningSelection(t, "provider", "model", ""), run.Capabilities{}, "lease-first", now)
	applied, err := store.Save(t.Context(), goalReplacement(t, first, unwrittenVersion(t, sessionID)))
	if err != nil || !applied {
		t.Fatalf("insert first goal: applied=%v err=%v", applied, err)
	}
	firstVersion := first.Version()
	first, err = first.Pause(goal.ReasonStoppedByUser, "", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	applied, err = store.Save(t.Context(), goalReplacement(t, first, firstVersion))
	if err != nil || !applied {
		t.Fatalf("stop first goal: applied=%v err=%v", applied, err)
	}

	fresh, _ := goal.New(sessionID, "second", testReasoningSelection(t, "provider", "model", ""), run.Capabilities{}, "lease-second", now.Add(2*time.Second))
	applied, err = store.Save(t.Context(), goalReplacement(t, fresh, first.Version()))
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

	stale, _ := goal.New(sessionID, "old", testReasoningSelection(t, "provider", "model", ""), run.Capabilities{}, "lease-old", now)
	staleVersion := stale.Version()
	if applied, err := store.Save(t.Context(), goalReplacement(t, stale, unwrittenVersion(t, sessionID))); err != nil || !applied {
		t.Fatalf("seed stale goal: applied=%v err=%v", applied, err)
	}
	if err := store.Clear(t.Context(), sessionID); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	fresh, _ := goal.New(sessionID, "new", testReasoningSelection(t, "provider", "model", ""), run.Capabilities{}, "lease-fresh", now)
	if applied, err := store.Save(t.Context(), goalReplacement(t, fresh, unwrittenVersion(t, sessionID))); err != nil || !applied {
		t.Fatalf("seed fresh goal: applied=%v err=%v", applied, err)
	}

	stale, _ = stale.Pause(goal.ReasonRunNotCompleted, "error", now)
	if applied, err := store.Save(t.Context(), goalReplacement(t, stale, staleVersion)); err != nil || applied {
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
	g, _ := goal.New("missing", "obj", testReasoningSelection(t, "provider", "model", ""), run.Capabilities{}, "lease-missing", time.Unix(0, 0))
	if applied, err := store.Save(t.Context(), goalReplacement(t, g, unwrittenVersion(t, "missing"))); err == nil || applied {
		t.Fatalf("Save(missing session) = applied=%v err=%v, want false/non-nil", applied, err)
	}
}

func TestGoalStoreCascadesWithSessionDeletion(t *testing.T) {
	store, sessions, _ := newGoalRunStores(t)
	const sessionID = "s"
	seedSession(t, sessions, sessionID)
	g, _ := goal.New(sessionID, "obj", testReasoningSelection(t, "provider", "model", ""), run.Capabilities{}, "lease", time.Unix(0, 0))
	if applied, err := store.Save(t.Context(), goalReplacement(t, g, unwrittenVersion(t, sessionID))); err != nil || !applied {
		t.Fatalf("seed goal: applied=%v err=%v", applied, err)
	}
	if err := sessions.Delete(t.Context(), sessionID); err != nil {
		t.Fatalf("Delete(session): %v", err)
	}
	if _, ok, err := readGoal(t.Context(), store, sessionID); err != nil || ok {
		t.Fatalf("goal after session delete = present=%v err=%v, want false/nil", ok, err)
	}
}

func TestGoalStoreExecutesDomainDecidedRevision(t *testing.T) {
	store, sessions := newGoalStore(t)
	const sessionID = "s"
	seedSession(t, sessions, sessionID)
	g, _ := goal.New(sessionID, "obj", testReasoningSelection(t, "provider", "model", ""), run.Capabilities{}, "lease", time.Unix(0, 0))
	applied, err := store.Save(t.Context(), goalReplacement(t, g, unwrittenVersion(t, sessionID)))
	if err != nil || !applied || g.Revision() != 1 {
		t.Fatalf("insert = revision %d, applied=%v err=%v, want 1/true/nil", g.Revision(), applied, err)
	}

	invalidSnapshot := g.Snapshot()
	invalidSnapshot.Revision = 99
	invalidAdvance, restoreErr := goal.Restore(invalidSnapshot)
	if restoreErr != nil {
		t.Fatal(restoreErr)
	}
	if _, err := goal.NewReplacement(g.Version(), invalidAdvance); err == nil {
		t.Fatal("NewReplacement accepted a non-advancing Goal")
	}
	if applied, err := store.Save(t.Context(), goal.Replacement{}); err == nil || applied {
		t.Fatalf("Save(zero replacement) = applied=%v err=%v, want false/non-nil", applied, err)
	}

	updated, err := g.Pause(goal.ReasonStoppedByUser, "", time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	applied, err = store.Save(t.Context(), goalReplacement(t, updated, g.Version()))
	if err != nil || !applied || updated.Revision() != 2 {
		t.Fatalf("update = revision %d, applied=%v err=%v, want 2/true/nil", updated.Revision(), applied, err)
	}

	exhaustedSnapshot := updated.Snapshot()
	exhaustedSnapshot.Revision = math.MaxInt64
	exhausted, restoreErr := goal.Restore(exhaustedSnapshot)
	if restoreErr != nil {
		t.Fatal(restoreErr)
	}
	if _, err := goal.NewReplacement(exhausted.Version(), exhausted); err == nil {
		t.Fatal("NewReplacement accepted an exhausted Goal revision")
	}
}
