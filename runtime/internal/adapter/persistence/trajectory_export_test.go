package persistence

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type exportSnapshotReader struct {
	value sessions.Snapshot
	reads int
}

func (r *exportSnapshotReader) ReadSnapshot(context.Context, string) (sessions.Snapshot, error) {
	r.reads++
	return r.value, nil
}

func trajectoryExportFixture(t *testing.T) (*TrajectoryExportReader, *sql.DB, *exportSnapshotReader) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "export.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ses := testsupport.MustRestoreSession(session.Snapshot{ID: "ses_export"})
	if err := sqlite.NewSessionStore(db).Insert(t.Context(), ses); err != nil {
		t.Fatal(err)
	}
	run := testsupport.MustRestoreRun(run.Snapshot{ID: "run_export", SessionID: ses.ID(), State: run.Completed})
	if err := sqlite.NewRunStore(db).Restore(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	modelReader, err := NewModelInvocationReader(sqlite.NewModelInvocationStore(db))
	if err != nil {
		t.Fatal(err)
	}
	snapshots := &exportSnapshotReader{value: sessions.Snapshot{Session: ses}}
	snapshots.value.Runs = append(snapshots.value.Runs, run)
	reader, err := NewTrajectoryExportReader(TrajectoryExportConfig{
		Snapshots: snapshots, Sessions: sqlite.NewSessionStore(db), ModelInvocations: modelReader,
		ToolInvocations: sqlite.NewToolInvocationStore(db), Feedback: sqlite.NewFeedbackStore(db),
		Tx: func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return reader, db, snapshots
}

func TestTrajectoryExportRejectsResourceLimitsBeforeReadingBodies(t *testing.T) {
	for _, test := range []struct {
		name string
		sql  string
		args []any
	}{
		{
			name: "source bytes",
			sql:  `INSERT INTO feedback_entries(session_id,text,created_at) VALUES('ses_export',zeroblob(?),1)`,
			args: []any{sessions.MaximumTrajectoryExportBytes + 1},
		},
		{
			name: "feedback records",
			sql: `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x < ?)
				INSERT INTO feedback_entries(session_id,text,created_at) SELECT 'ses_export','evidence',1 FROM n`,
			args: []any{sessions.MaximumTrajectoryExportRecords + 1},
		},
		{
			name: "Tool attempt records",
			sql: `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x < ?)
				INSERT INTO tool_invocations(call_id,item_id,session_id,run_id,segment_id,state,started_at,finished_at)
				SELECT 'call_'||x,'item_'||x,'ses_export','run_export','seg_1','incomplete',1,2 FROM n`,
			args: []any{sessions.MaximumTrajectoryExportRecords + 1},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader, db, snapshots := trajectoryExportFixture(t)
			if _, err := db.ExecContext(t.Context(), test.sql, test.args...); err != nil {
				t.Fatal(err)
			}
			value, err := reader.ReadTrajectoryExport(t.Context(), "ses_export")
			if !errors.Is(err, sessions.ErrExportTooLarge) || snapshots.reads != 0 || value.Snapshot.Session.ID() != "" {
				t.Fatalf("oversized evidence was loaded: reads=%d, error=%v", snapshots.reads, err)
			}
		})
	}
}

func TestTrajectoryExportDiscardsEarlierPagesWhenLaterEvidenceIsCorrupt(t *testing.T) {
	reader, db, snapshots := trajectoryExportFixture(t)
	if _, err := db.ExecContext(t.Context(), `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x < 101)
		INSERT INTO model_invocations(call_id,session_id,run_id,segment_id,state,started_at,finished_at)
		SELECT printf('call_%03d',x),'ses_export','run_export',CASE WHEN x=1 THEN '' ELSE 'seg_1' END,'completed',?,? FROM n`,
		time.Unix(10, 0).UnixNano(), time.Unix(11, 0).UnixNano(),
	); err != nil {
		t.Fatal(err)
	}
	value, err := reader.ReadTrajectoryExport(t.Context(), "ses_export")
	if err == nil || snapshots.reads != 1 || value.Snapshot.Session.ID() != "" || len(value.ModelInvocations) != 0 {
		t.Fatalf("corrupt later page produced partial export: calls=%d, error=%v", len(value.ModelInvocations), err)
	}
}

func TestTrajectoryExportRejectsMismatchedJournalOwnershipBeforeLoading(t *testing.T) {
	for _, statement := range []string{
		`INSERT INTO model_invocations(call_id,session_id,run_id,segment_id,state,started_at,finished_at)
		 VALUES('call_foreign','ses_foreign','run_export','seg_1','completed',1,2)`,
		`INSERT INTO tool_invocations(call_id,item_id,session_id,run_id,segment_id,state,started_at,finished_at)
		 VALUES('call_foreign','item_1','ses_foreign','run_export','seg_1','incomplete',1,2)`,
	} {
		reader, db, snapshots := trajectoryExportFixture(t)
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
		value, err := reader.ReadTrajectoryExport(t.Context(), "ses_export")
		if err == nil || snapshots.reads != 0 || value.Snapshot.Session.ID() != "" {
			t.Fatalf("foreign journal bypassed source admission: reads=%d, error=%v", snapshots.reads, err)
		}
	}
}
