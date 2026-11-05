package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/testsupport/goaltest"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/frame/sqlite"
)

// The separate writer commits after the Goal row is read and before usage is
// read. WAL permits that commit even while the reader holds its old snapshot.
func TestGoalReadsKeepLifecycleAndUsageInOneSnapshot(t *testing.T) {
	for _, list := range []bool{false, true} {
		t.Run(fmt.Sprintf("list=%t", list), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "flame.db")
			writer, err := openDatabase(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = writer.Close() })
			goals := newGoalStore(databaseFor(writer))
			seedSession(t, newSessionStore(databaseFor(writer)), "ses_goal_snapshot")
			now := time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC)
			initial, err := goaltest.New("ses_goal_snapshot", "finish", testReasoningSelection(t, "provider", "model", ""), run.Capabilities{}, "goal_snapshot", now)
			if err != nil {
				t.Fatal(err)
			}
			if applied, err := goals.Save(t.Context(), goalReplacement(t, initial, unwrittenVersion(t, initial.SessionID()))); err != nil || !applied {
				t.Fatalf("save initial Goal = %t, %v", applied, err)
			}
			paused, err := goaltest.Paused(initial, goal.ReasonStoppedByUser, now.Add(time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			change := goalReplacement(t, paused, initial.Version())
			terminal := testsupport.MustRestoreRun(run.Snapshot{
				SessionID: initial.SessionID(), ID: "run_goal_snapshot", GoalIncarnationID: initial.IncarnationID(),
				Ending: run.Completed{}, CreatedAt: now, FinishedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute),
				Metrics: testsupport.MustRunMetrics(testsupport.RunMetricsInput{
					Steps: 3, Usage: &accounting.Usage{Total: accounting.Totals{Cost: testsupport.OptionalCost(new(0.25))}},
				}),
			})
			transition := func() error {
				return sqlite.RunInTx(t.Context(), writer, func(ctx context.Context) error {
					if err := newRunStore(databaseFor(writer)).Restore(ctx, terminal); err != nil {
						return err
					}
					applied, err := goals.Save(ctx, change)
					if err == nil && !applied {
						return fmt.Errorf("Goal change was not applied")
					}
					return err
				})
			}
			reader, err := openDatabase(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = reader.Close() })
			readStore := newGoalStore(&database{boundary: goalSnapshotBoundary{
				databaseBoundary: independentDatabase{db: reader}, afterGoalRead: transition,
			}})
			var observed goal.Goal
			if list {
				values, err := readStore.List(t.Context())
				if err != nil || len(values) != 1 {
					t.Fatalf("List = %+v, %v", values, err)
				}
				observed = values[0]
			} else {
				var found bool
				observed, found, err = readGoal(t.Context(), readStore, initial.SessionID())
				if err != nil || !found {
					t.Fatalf("Get = %t, %v", found, err)
				}
			}
			if observed.Status() != goal.StatusActive || observed.Used().Runs != 0 {
				t.Fatalf("read mixed the old Goal and new Runs: %+v", observed.Snapshot())
			}
			latest, found, err := readGoal(t.Context(), goals, initial.SessionID())
			if err != nil || !found || latest.Status() != goal.StatusPaused || latest.Used().Runs != 1 {
				t.Fatalf("latest Goal = %+v, %t, %v", latest.Snapshot(), found, err)
			}
		})
	}
}

type goalSnapshotBoundary struct {
	databaseBoundary
	afterGoalRead func() error
}

func (b goalSnapshotBoundary) read(ctx context.Context, read func(sqlite.Reader) error) error {
	return b.databaseBoundary.read(ctx, func(reader sqlite.Reader) error {
		return read(&goalSnapshotReader{Reader: reader, afterGoalRead: b.afterGoalRead})
	})
}

type goalSnapshotReader struct {
	sqlite.Reader
	afterGoalRead func() error
	changed       bool
}

func (r *goalSnapshotReader) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if !r.changed && strings.Contains(query, "FROM runs") {
		r.changed = true
		if err := r.afterGoalRead(); err != nil {
			return nil, err
		}
	}
	return r.Reader.QueryContext(ctx, query, args...)
}
