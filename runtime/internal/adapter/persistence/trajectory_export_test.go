package persistence

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	"github.com/Tangerg/frame/sqlite"
	"github.com/Tangerg/scope/core/chat"
)

type exportSnapshotReader struct {
	value sessions.Snapshot
	reads int
}

func (r *exportSnapshotReader) ReadSnapshot(context.Context, string) (sessions.Snapshot, error) {
	r.reads++
	return r.value, nil
}

func trajectoryExportFixture(t *testing.T) (*trajectoryExportReader, *sqlite.Database, *exportSnapshotReader) {
	t.Helper()
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "export.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ses := testsupport.MustRestoreSession(session.Snapshot{ID: "ses_export"})
	if err := newSessionStore(databaseFor(db)).Insert(t.Context(), ses); err != nil {
		t.Fatal(err)
	}
	run := testsupport.MustRestoreRun(run.Snapshot{ID: "run_export", SessionID: ses.ID(), State: run.Finished})
	if err := newRunStore(databaseFor(db)).Restore(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	modelReader, err := NewModelInvocationReader(newModelInvocationStore(databaseFor(db)))
	if err != nil {
		t.Fatal(err)
	}
	snapshots := &exportSnapshotReader{value: sessions.Snapshot{Session: ses}}
	snapshots.value.Runs = append(snapshots.value.Runs, run)
	reader, err := NewTrajectoryExportReader(TrajectoryExportConfig{
		Snapshots: snapshots, Sessions: newSessionStore(databaseFor(db)), ModelInvocations: modelReader,
		ToolInvocations: newToolInvocationStore(databaseFor(db)), Annotations: &annotationStore{db: databaseFor(db)},
		ReadSnapshot: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunReadSnapshot(ctx, db, fn)
		},
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
			sql:  `INSERT INTO session_annotations(command_id,installation,release,contribution,contract,session_id,run_id,item_id,content,created_at_ns) VALUES('large','source','digest','note','contract','ses_export','','','"'||hex(zeroblob(?))||'"',1)`,
			args: []any{sessions.MaximumTrajectoryExportBytes + 1},
		},
		{
			name: "endpoint failure bytes",
			sql:  `INSERT INTO model_invocations(call_id,session_id,state,started_at,finished_at,endpoint_json,endpoint_kind,endpoint_failure) VALUES('endpoint_large','ses_export','unknown',1,2,'{}','chat',zeroblob(?))`,
			args: []any{sessions.MaximumTrajectoryExportBytes + 1},
		},
		{
			name: "annotation records",
			sql: `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x < ?)
				INSERT INTO session_annotations(command_id,installation,release,contribution,contract,session_id,run_id,item_id,content,created_at_ns) SELECT x,'source','digest','note','contract','ses_export','','','{}',1 FROM n`,
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
			if _, err := execFixture(t.Context(), db, test.sql, test.args...); err != nil {
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
	if _, err := execFixture(t.Context(), db, `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x < 101)
		INSERT INTO model_invocations(call_id,session_id,run_id,segment_id,state,started_at,finished_at,logical_purpose)
		SELECT printf('call_%03d',x),'ses_export','run_export',CASE WHEN x=1 THEN '' ELSE 'seg_1' END,'completed',?,?,'conversation' FROM n`, time.Unix(10, 0).UnixNano(), time.Unix(11, 0).UnixNano()); err != nil {
		t.Fatal(err)
	}
	value, err := reader.ReadTrajectoryExport(t.Context(), "ses_export")
	if err == nil || snapshots.reads != 1 || value.Snapshot.Session.ID() != "" || len(value.ModelInvocations) != 0 {
		t.Fatalf("corrupt later page produced partial export: calls=%d, error=%v", len(value.ModelInvocations), err)
	}
}

func TestTrajectoryExportRejectsMismatchedJournalOwnershipBeforeLoading(t *testing.T) {
	for _, statement := range []string{
		`INSERT INTO model_invocations(call_id,session_id,run_id,segment_id,state,started_at,finished_at,logical_purpose)
		 VALUES('call_foreign','ses_foreign','run_export','seg_1','completed',1,2,'conversation')`,
		`INSERT INTO tool_invocations(call_id,item_id,session_id,run_id,segment_id,state,started_at,finished_at)
		 VALUES('call_foreign','item_1','ses_foreign','run_export','seg_1','incomplete',1,2)`,
	} {
		reader, db, snapshots := trajectoryExportFixture(t)
		if _, err := execFixture(t.Context(), db, statement); err != nil {
			t.Fatal(err)
		}
		value, err := reader.ReadTrajectoryExport(t.Context(), "ses_export")
		if err == nil || snapshots.reads != 0 || value.Snapshot.Session.ID() != "" {
			t.Fatalf("foreign journal bypassed source admission: reads=%d, error=%v", snapshots.reads, err)
		}
	}
}

func TestTrajectoryExportIncludesSessionOnlyKnownEndpointEvidence(t *testing.T) {
	reader, db, _ := trajectoryExportFixture(t)
	store := newModelInvocationStore(databaseFor(db))
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original prompt")))
	message := chat.NewAssistantMessage(chat.NewTextPart("known answer"))
	response := &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: 9007199254740993, OutputTokens: 2}}}
	for index, scope := range []string{"ses_export", ""} {
		id := []string{"session", "global"}[index]
		invocation := modelref.EndpointInvocation{ID: "endpoint_" + id, Caller: framemcp.Invocation{ID: "utility_" + id, Origin: extension.Automatic, Parent: "maintenance", Target: portable.Target{Scope: extension.ScopeCall, Reference: "utility_" + id}}, Provider: "anthropic", Model: "captured-model", SessionID: scope, Request: modelref.ChatEndpointRequest{Chat: request}, StartedAt: time.Unix(100, 0).UTC()}
		if err := store.StartEndpointInvocation(t.Context(), invocation, ""); err != nil {
			t.Fatal(err)
		}
		if err := store.SettleEndpointInvocation(t.Context(), invocation, modelref.EndpointOutcome{Response: modelref.ChatEndpointResponse{Chat: response}, FinishedAt: invocation.StartedAt.Add(time.Second), Failure: "outer rejected text"}); err != nil {
			t.Fatal(err)
		}
	}
	evidence, err := reader.ReadTrajectoryExport(t.Context(), "ses_export")
	if err != nil || len(evidence.ModelEndpoints) != 1 {
		t.Fatalf("Session endpoint export: %+v %v", evidence.ModelEndpoints, err)
	}
	value := evidence.ModelEndpoints[0]
	if value.Snapshot().SessionID != "ses_export" || value.Snapshot().RunID != "" || value.State() != modelref.InvocationFailed || value.Response().Metadata.Usage.InputTokens != 9007199254740993 || !strings.Contains(value.Snapshot().ResponseJSON.String(), "9007199254740993") || !strings.Contains(value.Snapshot().RequestJSON.String(), "original prompt") {
		t.Fatalf("export changed known endpoint evidence: %+v", value)
	}
	if _, err := execFixture(t.Context(), db, `UPDATE model_invocations SET endpoint_json=json_set(endpoint_json,'$.sessionId','ses_other') WHERE call_id='endpoint_session'`); err != nil {
		t.Fatal(err)
	}
	if value, err := reader.ReadTrajectoryExport(t.Context(), "ses_export"); err == nil || len(value.ModelEndpoints) != 0 {
		t.Fatal("corrupt Session association exported partial evidence")
	}
}
