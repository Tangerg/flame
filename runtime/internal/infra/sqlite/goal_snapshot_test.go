package sqlite_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

// The separate writer commits after the Goal row is read and before usage is
// read. WAL permits that commit even while the reader holds its old snapshot.
func TestGoalReadsKeepLifecycleAndUsageInOneSnapshot(t *testing.T) {
	for _, list := range []bool{false, true} {
		t.Run(fmt.Sprintf("list=%t", list), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "flame.db")
			writer, err := sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = writer.Close() })
			goals := sqlite.NewGoalStore(writer)
			seedSession(t, sqlite.NewSessionStore(writer), "ses_goal_snapshot")
			now := time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC)
			initial, err := goal.New("ses_goal_snapshot", "finish", testReasoningSelection(t, "provider", "model", ""), run.Capabilities{}, "goal_snapshot", now)
			if err != nil {
				t.Fatal(err)
			}
			if applied, err := goals.Save(t.Context(), goalReplacement(t, initial, unwrittenVersion(t, initial.SessionID()))); err != nil || !applied {
				t.Fatalf("save initial Goal = %t, %v", applied, err)
			}
			paused, err := initial.Pause(goal.ReasonStoppedByUser, "", now.Add(time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			change := goalReplacement(t, paused, initial.Version())
			terminal := testsupport.MustRestoreRun(run.Snapshot{
				SessionID: initial.SessionID(), ID: "run_goal_snapshot", GoalIncarnationID: initial.IncarnationID(),
				Outcome: new(run.OutcomeCompleted), CreatedAt: now, FinishedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute),
				Metrics: testsupport.MustRunMetrics(testsupport.RunMetricsInput{
					Steps: 3, Usage: &accounting.Usage{Total: accounting.Totals{CostUSD: new(0.25)}},
				}),
			})
			snapshotDriver := goalSnapshotDriver{
				Driver: writer.Driver(),
				afterGoalRead: func() error {
					return sqlite.RunInTx(t.Context(), writer, func(ctx context.Context) error {
						if err := sqlite.NewRunStore(writer).Restore(ctx, terminal); err != nil {
							return err
						}
						applied, err := goals.Save(ctx, change)
						if err == nil && !applied {
							return fmt.Errorf("Goal change was not applied")
						}
						return err
					})
				},
			}
			reader := sql.OpenDB(goalSnapshotConnector{
				driver: snapshotDriver,
				dsn:    "file:" + url.PathEscape(filepath.ToSlash(path)) + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)",
			})
			reader.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = reader.Close() })
			readStore := sqlite.NewGoalStore(reader)
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

type goalSnapshotDriver struct {
	driver.Driver
	afterGoalRead func() error
}

type goalSnapshotConnector struct {
	driver goalSnapshotDriver
	dsn    string
}

func (c goalSnapshotConnector) Connect(context.Context) (driver.Conn, error) {
	return c.driver.Open(c.dsn)
}

func (c goalSnapshotConnector) Driver() driver.Driver { return c.driver }

func (d goalSnapshotDriver) Open(name string) (driver.Conn, error) {
	connection, err := d.Driver.Open(name)
	if err != nil {
		return nil, err
	}
	return &goalSnapshotConnection{Conn: connection, afterGoalRead: d.afterGoalRead}, nil
}

type goalSnapshotConnection struct {
	driver.Conn
	afterGoalRead func() error
}

func (c *goalSnapshotConnection) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
	if err != nil || !strings.Contains(query, "FROM goals") {
		return rows, err
	}
	return &goalSnapshotRows{Rows: rows, afterGoalRead: c.afterGoalRead}, nil
}

type goalSnapshotRows struct {
	driver.Rows
	afterGoalRead func() error
	once          sync.Once
	closeError    error
}

func (r *goalSnapshotRows) Close() error {
	r.once.Do(func() {
		r.closeError = r.Rows.Close()
		if r.closeError == nil {
			r.closeError = r.afterGoalRead()
		}
	})
	return r.closeError
}
