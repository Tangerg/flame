package persistence

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type blockingGoalProjection struct {
	*sqlite.GoalStore
	entered chan struct{}
	release chan struct{}
}

func (b *blockingGoalProjection) Get(ctx context.Context, sessionID string) (goal.Current, error) {
	close(b.entered)
	select {
	case <-b.release:
	case <-ctx.Done():
		return goal.Current{}, ctx.Err()
	}
	return b.GoalStore.Get(ctx, sessionID)
}

func TestReadMaterialSnapshotKeepsSessionPlanAndGoalOnOneTransaction(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "runtime.db")
	readerDB, err := sqlite.Open(t.Context(), databasePath)
	if err != nil {
		t.Fatalf("open reader database: %v", err)
	}
	t.Cleanup(func() { _ = readerDB.Close() })
	writerDB, err := sqlite.Open(t.Context(), databasePath)
	if err != nil {
		t.Fatalf("open writer database: %v", err)
	}
	t.Cleanup(func() { _ = writerDB.Close() })
	ctx := t.Context()
	createdAt := time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)
	readerSessionStore := sqlite.NewSessionStore(readerDB)
	writerSessionStore := sqlite.NewSessionStore(writerDB)
	original := testsupport.MustRestoreSession(session.Snapshot{
		ID: "ses_snapshot", Workspace: testsupport.MustWorkspace("/workspace"), Title: "before",
		StartedAt: createdAt, UpdatedAt: createdAt, Revision: 1,
	})
	if insertErr := writerSessionStore.Insert(ctx, original); insertErr != nil {
		t.Fatalf("seed Session: %v", insertErr)
	}
	readerPlanStore := sqlite.NewPlanStore(readerDB)
	writerPlanStore := sqlite.NewPlanStore(writerDB)
	originalPlan, err := (plan.Current{}).Replace([]plan.Step{{
		Description: "before", Status: plan.StatusInProgress,
	}}, createdAt)
	if err != nil {
		t.Fatalf("prepare Plan: %v", err)
	}
	originalPlanReplacement, err := plan.NewReplacement((plan.Current{}).Version(), originalPlan)
	if err != nil {
		t.Fatalf("prepare initial Plan replacement: %v", err)
	}
	if saveErr := writerPlanStore.Save(ctx, original.ID(), originalPlanReplacement); saveErr != nil {
		t.Fatalf("seed Plan: %v", saveErr)
	}
	readerGoalStore := sqlite.NewGoalStore(readerDB)
	writerGoalStore := sqlite.NewGoalStore(writerDB)
	selection, err := modelref.New("provider", "model")
	if err != nil {
		t.Fatal(err)
	}
	originalGoal, err := goal.New(
		original.ID(), "before", selection, goal.UnlimitedBudget(), run.Capabilities{},
		"goal_before", createdAt,
	)
	if err != nil {
		t.Fatalf("prepare Goal: %v", err)
	}
	unwritten, err := goal.Unwritten(original.ID())
	if err != nil {
		t.Fatal(err)
	}
	goalReplacement, err := goal.NewReplacement(unwritten.Version(), originalGoal)
	if err != nil {
		t.Fatalf("prepare initial Goal replacement: %v", err)
	}
	applied, err := writerGoalStore.Save(ctx, goalReplacement)
	if err != nil || !applied {
		t.Fatalf("seed Goal: applied=%t err=%v", applied, err)
	}
	blockingGoal := &blockingGoalProjection{
		GoalStore: readerGoalStore, entered: make(chan struct{}), release: make(chan struct{}),
	}
	stores := NewSessionStores(SessionStoresConfig{
		Sessions: readerSessionStore, Transcript: sqlite.NewTranscriptStore(readerDB),
		Interrupts: NewInterruptStore(sqlite.NewInterruptStore(readerDB)),
		Runs:       sqlite.NewRunStore(readerDB), Plan: readerPlanStore, Goals: blockingGoal,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, readerDB, fn)
		},
	})

	snapshotResult := make(chan struct {
		snapshotRevision uint64
		planRevision     uint64
		goalRevision     int64
		err              error
	}, 1)
	go func() {
		snapshot, readMaterialSnapshotErr := stores.ReadMaterialSnapshot(ctx, original.ID())
		committedPlan, committed := snapshot.Plan.State()
		if readMaterialSnapshotErr == nil && !committed {
			readMaterialSnapshotErr = errors.New("snapshot Plan is unwritten")
		}
		snapshotResult <- struct {
			snapshotRevision uint64
			planRevision     uint64
			goalRevision     int64
			err              error
		}{snapshot.Session.Revision(), committedPlan.Revision(), snapshot.Goal.Revision(), readMaterialSnapshotErr}
	}()
	<-blockingGoal.entered

	updatedTitle := "after"
	updatedSession, changed, err := original.Apply(session.Patch{
		Title: &updatedTitle, ExpectedRevision: original.Revision(),
	}, createdAt.Add(time.Second))
	if err != nil || !changed {
		t.Fatalf("prepare Session replacement: changed=%t err=%v", changed, err)
	}
	updatedPlan, err := originalPlan.Replace([]plan.Step{{
		Description: "after", Status: plan.StatusCompleted,
	}}, createdAt.Add(time.Second))
	if err != nil {
		t.Fatalf("prepare Plan replacement: %v", err)
	}
	updatedGoal, err := originalGoal.Pause(goal.ReasonRuntimeRestarted, "", createdAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- sqlite.RunInTx(ctx, writerDB, func(ctx context.Context) error {
			replacement, replacementErr := session.NextReplacement(original, updatedSession)
			if replacementErr != nil {
				return replacementErr
			}
			if saveErr := writerSessionStore.Save(ctx, replacement); saveErr != nil {
				return saveErr
			}
			originalCurrent, currentErr := plan.CurrentOf(originalPlan)
			if currentErr != nil {
				return currentErr
			}
			planReplacement, replacementErr := plan.NewReplacement(originalCurrent.Version(), updatedPlan)
			if replacementErr != nil {
				return replacementErr
			}
			if saveErr := writerPlanStore.Save(ctx, original.ID(), planReplacement); saveErr != nil {
				return saveErr
			}
			goalReplacement, replacementErr := goal.NewReplacement(originalGoal.Version(), updatedGoal)
			if replacementErr != nil {
				return replacementErr
			}
			applied, saveErr := writerGoalStore.Save(ctx, goalReplacement)
			if saveErr == nil && !applied {
				return errors.New("replace Goal: CAS did not apply")
			}
			return saveErr
		})
	}()
	if writerErr := <-writerDone; writerErr != nil {
		t.Fatalf("commit concurrent successor state: %v", writerErr)
	}
	close(blockingGoal.release)

	read := <-snapshotResult
	if read.err != nil {
		t.Fatalf("ReadMaterialSnapshot: %v", read.err)
	}
	if read.snapshotRevision != 1 || read.planRevision != 1 || read.goalRevision != 1 {
		t.Fatalf(
			"snapshot revisions = Session:%d Plan:%d Goal:%d, want 1/1/1",
			read.snapshotRevision, read.planRevision, read.goalRevision,
		)
	}
	stores.goals = readerGoalStore
	after, err := stores.ReadMaterialSnapshot(ctx, original.ID())
	if err != nil {
		t.Fatalf("read successor snapshot: %v", err)
	}
	afterPlan, committed := after.Plan.State()
	if !committed || after.Session.Revision() != 2 || afterPlan.Revision() != 2 || after.Goal.Revision() != 2 {
		t.Fatalf(
			"successor revisions = Session:%d Plan:%d Goal:%d, want 2/2/2",
			after.Session.Revision(), afterPlan.Revision(), after.Goal.Revision(),
		)
	}
}
