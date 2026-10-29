package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func insertTrajectorySession(t testing.TB, ctx context.Context, db *sql.DB, id string) {
	t.Helper()
	value := testsupport.MustRestoreSession(session.Snapshot{
		ID: id, Workspace: testsupport.MustWorkspace("/work"),
		CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC(), Revision: 1,
	})
	if err := newSessionStore(db).Insert(ctx, value); err != nil {
		t.Fatal(err)
	}
}

func trajectoryItem(id, runID, sessionID string, at time.Time) transcript.Item {
	return testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: id, RunID: runID, SessionID: sessionID, OccurredAt: at,
		Kind: transcript.UserMessage, Status: transcript.ItemCompleted,
	})
}

func positionOfTrajectory(row trajectoryRecord) trajectoryPosition {
	position := trajectoryPosition{OccurredAt: row.OccurredAt.UnixNano()}
	switch {
	case row.Run != nil:
		position.Kind, position.ID = "run", row.Run.ID()
	case row.Model != nil:
		position.Kind, position.ID = "model", row.Model.CallID
	case row.Endpoint != nil:
		position.Kind, position.ID = "endpoint", row.Endpoint.Invocation.ID
	case row.Item != nil:
		position.Kind, position.ID = "item", row.Item.ID()
	}
	return position
}

func TestSessionTrajectorySurvivesRestartAndPagesSameTimeSourcesWithoutLoss(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "trajectory.db")
	db, err := openDatabase(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	draft := runDraft("run_trajectory", "ses_trajectory")
	insertTrajectorySession(t, ctx, db, draft.SessionID)
	runStore := newRunStore(db)
	if err := runStore.Admit(ctx, draft); err != nil {
		t.Fatal(err)
	}
	models := newModelInvocationStore(db)
	items := newTranscriptStore(db)
	for index := range 151 {
		id := fmt.Sprintf("call_%03d", index)
		if err := models.StartModelInvocation(ctx, draft.SessionID, draft.RunID, draft.SegmentID, id, draft.CreatedAt, run.ModelConversation); err != nil {
			t.Fatal(err)
		}
		if index == 150 {
			err = models.MarkModelInvocationUnknown(ctx, draft.SessionID, draft.RunID, draft.SegmentID, id, draft.CreatedAt, draft.CreatedAt.Add(time.Second))
		} else {
			err = models.CompleteModelInvocation(ctx, draft.SessionID, draft.RunID, draft.SegmentID, id, draft.CreatedAt, draft.CreatedAt.Add(time.Second), nil, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := items.AppendItem(ctx, trajectoryItem(fmt.Sprintf("item_%03d", index), draft.RunID, draft.SessionID, draft.CreatedAt)); err != nil {
			t.Fatal(err)
		}
	}
	if err := runStore.Terminalize(ctx, storedRunReplacement(t, ctx, runStore, finishedRunFromDraft(draft, run.OutcomeCompleted))); err != nil {
		t.Fatal(err)
	}
	reader := newTrajectoryStore(db)
	before, err := reader.Page(ctx, draft.SessionID, true, nil, 100)
	if err != nil || len(before) != 100 {
		t.Fatalf("before restart page: %d, %v", len(before), err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = openDatabase(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	reader = newTrajectoryStore(db)
	after, err := reader.Page(ctx, draft.SessionID, true, nil, 100)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("restart changed durable trajectory: %v", err)
	}
	seen := make(map[string]bool)
	var anchor *trajectoryPosition
	unknown := false
	for {
		page, err := reader.Page(ctx, draft.SessionID, true, anchor, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		for _, row := range page {
			position := positionOfTrajectory(row)
			key := position.Kind + ":" + position.ID
			if seen[key] {
				t.Fatalf("repeated observation %s", key)
			}
			seen[key] = true
			if row.Model != nil && row.Model.State == "unknown" {
				unknown = true
				if row.Model.Usage != nil || row.Model.FirstOutputLatencyMillis != nil {
					t.Fatalf("unknown observation invented metrics: %+v", row.Model)
				}
			}
		}
		anchor = new(positionOfTrajectory(page[len(page)-1]))
	}
	if len(seen) != 303 || !unknown {
		t.Fatalf("paged %d observations, unknown=%t", len(seen), unknown)
	}
	if _, err := reader.Page(ctx, "ses_absent", true, nil, 100); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("missing Session result = %v", err)
	}
}

func TestSessionTrajectoryKeepsDescendantAndSessionOwnership(t *testing.T) {
	ctx := t.Context()
	db, err := openDatabase(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	insertTrajectorySession(t, ctx, db, "ses_tree")
	insertTrajectorySession(t, ctx, db, "ses_other")
	root := runDraft("run_root", "ses_tree")
	root.Capabilities.ChildRuns = true
	child := runDraft("run_child", "ses_tree")
	child.ParentRunID, child.RootRunID, child.SpawnedByItemID = root.RunID, root.RunID, "item_spawn"
	other := runDraft("run_other", "ses_other")
	for _, draft := range []run.Draft{root, child, other} {
		if err := newRunStore(db).Admit(ctx, draft); err != nil {
			t.Fatal(err)
		}
		if err := newModelInvocationStore(db).StartModelInvocation(ctx, draft.SessionID, draft.RunID, draft.SegmentID, "call_"+draft.RunID, draft.CreatedAt, run.ModelConversation); err != nil {
			t.Fatal(err)
		}
		if err := newTranscriptStore(db).AppendItem(ctx, trajectoryItem("item_"+draft.RunID, draft.RunID, draft.SessionID, draft.CreatedAt)); err != nil {
			t.Fatal(err)
		}
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("captured")))
	for _, source := range []struct{ id, caller, session string }{
		{"endpoint_root", "call_run_root", ""}, {"endpoint_child", "call_run_child", ""},
		{"endpoint_session", "utility_session", root.SessionID}, {"endpoint_other", "utility_other", other.SessionID},
		{"endpoint_global", "utility_global", ""},
	} {
		value := modelref.EndpointInvocation{ID: source.id, Kind: modelref.EndpointChat, Caller: portable.Invocation{ID: source.caller, Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeCall, Reference: source.caller}}, Provider: "anthropic", Model: "captured-model", SessionID: source.session, Request: request, StartedAt: root.CreatedAt}
		owner := ""
		if source.caller == "call_run_root" {
			owner = source.caller
			value.SessionID = root.SessionID
			value.StartedAt = root.CreatedAt
		}
		if source.caller == "call_run_child" {
			owner = source.caller
			value.SessionID = child.SessionID
			value.StartedAt = child.CreatedAt
		}
		if err := newModelInvocationStore(db).StartEndpointInvocation(ctx, value, owner); err != nil {
			t.Fatal(err)
		}
	}
	reader := newTrajectoryStore(db)
	for _, includeDescendants := range []bool{false, true} {
		page, err := reader.Page(ctx, root.SessionID, includeDescendants, nil, 100)
		want := 5
		if includeDescendants {
			want = 9
		}
		if err != nil || len(page) != want {
			t.Fatalf("descendants=%t: %d entries, %v", includeDescendants, len(page), err)
		}
		for _, row := range page {
			if row.RunID == other.RunID || (!includeDescendants && row.RunID != "" && row.RunID != root.RunID) {
				t.Fatalf("out-of-scope trajectory entry: %+v", row)
			}
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE model_invocations SET session_id = ? WHERE run_id = ?`, root.SessionID, other.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Page(ctx, root.SessionID, true, nil, 100); err == nil {
		t.Fatal("corrupt cross-Session model observation was accepted")
	}
}

func BenchmarkSessionTrajectoryPage(b *testing.B) {
	for _, count := range []int{1000, 100000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			ctx := b.Context()
			db, err := openDatabase(ctx, ":memory:")
			if err != nil {
				b.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			insertTrajectorySession(b, ctx, db, "ses_bench")
			draft := runDraft("run_bench", "ses_bench")
			if err := newRunStore(db).Admit(ctx, draft); err != nil {
				b.Fatal(err)
			}
			if err := newTranscriptStore(db).AppendItem(ctx, trajectoryItem("item_template", draft.RunID, draft.SessionID, draft.CreatedAt)); err != nil {
				b.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `WITH RECURSIVE numbers(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM numbers WHERE n < ?)
				INSERT INTO history_items(session_id, run_id, item_id, occurred_at, payload, offload_id)
				SELECT session_id, run_id, printf('item_%06d', n), occurred_at + n, payload, '' FROM numbers CROSS JOIN history_items WHERE item_id = 'item_template'`, count/2); err != nil {
				b.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `WITH RECURSIVE numbers(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM numbers WHERE n < ?)
				INSERT INTO model_invocations(call_id, session_id, run_id, segment_id, state, started_at, finished_at)
				SELECT printf('call_%06d', n), 'ses_bench', 'run_bench', 'seg_open', 'completed', 1000000000 + n, 1000000001 + n FROM numbers`, count/2); err != nil {
				b.Fatal(err)
			}
			reader := newTrajectoryStore(db)
			for b.Loop() {
				rows, err := reader.Page(ctx, draft.SessionID, true, nil, 100)
				if err != nil || len(rows) != 100 {
					b.Fatalf("page = %d, %v", len(rows), err)
				}
			}
		})
	}
}
