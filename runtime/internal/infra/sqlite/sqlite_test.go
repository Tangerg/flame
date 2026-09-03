package sqlite_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	resultoffload "github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/exactint"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func TestOpenHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "flame.db")); !errors.Is(err, context.Canceled) {
		if db != nil {
			_ = db.Close()
		}
		t.Fatalf("Open error = %v, want context.Canceled", err)
	}
}

func newTempDB(t *testing.T) *sqlite.SessionStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewSessionStore(db)
}

func TestSessionSchemaOwnsExactWorkspacePath(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	rows, err := db.Query(`PRAGMA table_info(sessions)`)
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer func() { _ = rows.Close() }()
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table_info: %v", err)
	}
	if !columns["workspace_path"] || columns["cwd"] {
		t.Fatalf("sessions columns = %v, want workspace_path without cwd", columns)
	}

	const insert = `INSERT INTO sessions(
		id, title, title_search, workspace_path, workspace_search, parent_id, started_at, updated_at,
		provider, model, favorite, isolated, revision
	) VALUES (?, '', '', ?, ?, '', 1, 1, 'provider', 'model', 0, 0, 1)`
	if _, err := db.Exec(insert, "ses_empty", "", ""); err == nil {
		t.Fatal("sessions accepted an empty workspace_path")
	}
	if _, err := db.Exec(insert, "ses_relative", "relative/work", "relative/work"); err != nil {
		t.Fatalf("seed relative workspace: %v", err)
	}
	if _, err := sqlite.NewSessionStore(db).Get(t.Context(), "ses_relative"); !errors.Is(err, session.ErrInvalid) {
		t.Fatalf("decode relative workspace error = %v, want session.ErrInvalid", err)
	}
}

// TestSessionCRUD exercises the exact aggregate persistence lifecycle
// against the SQLite backend.
func TestSessionCRUD(t *testing.T) {
	ctx := context.Background()
	svc := newTempDB(t)

	// empty list at startup
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List on empty DB = %d entries", len(list))
	}

	selection, selectionErr := modelref.NewWithReasoningEffort("openai", "gpt-5.6-sol", "high")
	if selectionErr != nil {
		t.Fatalf("model selection: %v", selectionErr)
	}
	created := testsupport.MustRestoreSession(session.Snapshot{
		ID: "ses_first", Title: "first session", Workspace: testsupport.MustWorkspace("/work"),
		Selection: selection,
	})
	if insertErr := svc.Insert(ctx, created); insertErr != nil {
		t.Fatalf("Insert: %v", insertErr)
	}

	// get
	got, err := svc.Get(ctx, created.ID())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title() != "first session" {
		t.Fatalf("Get title = %q", got.Title())
	}
	if got.Selection() != selection {
		t.Fatalf("Get selection = %+v, want %+v", got.Selection(), selection)
	}
	if !got.UpdatedAt().Equal(created.UpdatedAt()) {
		t.Fatalf("UpdatedAt round-trip mismatch: got %v want %v", got.UpdatedAt(), created.UpdatedAt())
	}

	// list now has one
	list, err = svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID() != created.ID() {
		t.Fatalf("List = %+v", list)
	}

	// delete
	if err := svc.Delete(ctx, created.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// idempotent delete
	if err := svc.Delete(ctx, created.ID()); err != nil {
		t.Fatalf("Delete idempotent: %v", err)
	}

	// get after delete
	if _, err := svc.Get(ctx, created.ID()); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
}

// TestSessionPersistAcrossReopen confirms data survives a DB close +
// reopen — durability is the whole point of moving off in-memory.
func TestSessionPersistAcrossReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "flame.db")

	db1, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open 1: %v", err)
	}
	svc1 := sqlite.NewSessionStore(db1)
	created := testsupport.MustRestoreSession(session.Snapshot{
		ID: "ses_persistent", Title: "persistent", Workspace: testsupport.MustWorkspace("/work"),
	})
	if insertErr := svc1.Insert(ctx, created); insertErr != nil {
		t.Fatalf("Insert: %v", insertErr)
	}
	_ = db1.Close()

	db2, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open 2: %v", err)
	}
	defer func() { _ = db2.Close() }()
	svc2 := sqlite.NewSessionStore(db2)

	got, err := svc2.Get(ctx, created.ID())
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got.Title() != "persistent" {
		t.Fatalf("title = %q", got.Title())
	}
}

// TestMessageStore_RoundTrip exercises the conversation message store: append-order
// reads, per-conversation scoping, and Clear. Empty conversation reads as
// an empty slice; Clear is idempotent.
func TestMessageStore_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMessageStore(db)
	ctx := context.Background()

	var got []chat.Message
	got, err = store.Read(ctx, "conv-a")
	if err != nil || len(got) != 0 {
		t.Fatalf("Read empty = %v (err %v), want empty", got, err)
	}

	err = store.Write(ctx, "conv-a", chat.NewUserMessage(chat.NewTextPart("hello")), chat.NewAssistantMessage(chat.NewTextPart("hi")))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	err = store.Write(ctx, "conv-a", chat.NewUserMessage(chat.NewTextPart("again")))
	if err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	err = store.Write(ctx, "conv-b", chat.NewUserMessage(chat.NewTextPart("other")))
	if err != nil {
		t.Fatalf("Write conv-b: %v", err)
	}

	got, err = store.Read(ctx, "conv-a")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("conv-a len = %d, want 3 (append order across writes)", len(got))
	}
	if got[0].Role != chat.RoleUser || got[0].Text() != "hello" {
		t.Fatalf("got[0] = %#v, want user 'hello'", got[0])
	}
	if got2, _ := store.Read(ctx, "conv-b"); len(got2) != 1 {
		t.Fatalf("conv-b len = %d, want 1 (per-conversation scoping)", len(got2))
	}

	if err := store.Clear(ctx, "conv-a"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if got, _ := store.Read(ctx, "conv-a"); len(got) != 0 {
		t.Fatalf("after Clear conv-a len = %d, want 0", len(got))
	}
	if got2, _ := store.Read(ctx, "conv-b"); len(got2) != 1 {
		t.Fatalf("Clear leaked into conv-b: len = %d, want 1", len(got2))
	}
	if err := store.Clear(ctx, "conv-a"); err != nil {
		t.Fatalf("Clear idempotent: %v", err)
	}
}

// TestTranscriptStore_RoundTrip pins the item log: append order (ORDER BY seq)
// and per-session scoping. The Runs those items belong to are the run store's.
func TestTranscriptStore_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewTranscriptStore(db)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, it := range []transcript.Item{testsupport.MustRestoreItem(testsupport.ItemInput{SessionID: "ses_a", RunID: "run_1", ID: "i1", OccurredAt: now, Status: transcript.ItemCompleted, Kind: transcript.UserMessage, Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "one"}}}), testsupport.MustRestoreItem(testsupport.ItemInput{SessionID: "ses_a", RunID: "run_1", ID: "i2", OccurredAt: now, Status: transcript.ItemCompleted, Kind: transcript.AgentMessage, MessagePhase: transcript.MessageCommentary, Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "two"}}}), testsupport.MustRestoreItem(testsupport.ItemInput{SessionID: "ses_b", RunID: "run_9", ID: "i9", OccurredAt: now, Status: transcript.ItemCompleted, Kind: transcript.Reasoning, Text: "other"})} {
		err = store.AppendItem(ctx, it)
		if err != nil {
			t.Fatalf("append %s: %v", it.ID(), err)
		}
	}
	items, err := store.List(ctx, "ses_a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 || items[0].ID() != "i1" || items[1].ID() != "i2" ||
		items[1].MessagePhase() != transcript.MessageCommentary || items[1].Content()[0].Text != "two" {
		t.Fatalf("items = %+v, want [i1 i2]", items)
	}
}

func TestTranscriptStoreRejectsIdentityReparenting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewTranscriptStore(db)
	runs := sqlite.NewRunStore(db)
	ctx := t.Context()
	now := time.Now().UTC()

	if admitErr := runs.Admit(ctx, testsupport.RunDraft(run.Draft{SegmentID: "seg_open", RunID: "run_shared", SessionID: "ses_a", CreatedAt: now})); admitErr != nil {
		t.Fatalf("seed run: %v", admitErr)
	}
	if appendItemErr := store.AppendItem(ctx, testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID: "ses_a", RunID: "run_shared", ID: "item_shared", OccurredAt: now,
	})); appendItemErr != nil {
		t.Fatalf("seed item: %v", appendItemErr)
	}

	// A run id belongs to one session for its whole lifetime — and the refusal must
	// say so, not report the innocent session as busy.
	if admitErr := runs.Admit(ctx, testsupport.RunDraft(run.Draft{SegmentID: "seg_open", RunID: "run_shared", SessionID: "ses_b", CreatedAt: now})); !errors.Is(admitErr, run.ErrIdentityConflict) {
		t.Fatalf("re-parent run error = %v, want ErrIdentityConflict", admitErr)
	}
	if appendItemErr := store.AppendItem(ctx, testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID: "ses_b", RunID: "run_other", ID: "item_shared", OccurredAt: now,
	})); !errors.Is(appendItemErr, transcript.ErrIdentityConflict) {
		t.Fatalf("re-parent item error = %v, want ErrIdentityConflict", appendItemErr)
	}
	if appendItemErr := store.AppendItem(ctx, testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID: "ses_a", RunID: "run_shared", ID: "item_shared", OccurredAt: now.Add(time.Second),
	})); !errors.Is(appendItemErr, transcript.ErrIdentityConflict) {
		t.Fatalf("move item occurrence error = %v, want ErrIdentityConflict", appendItemErr)
	}

	itemsA, err := store.List(ctx, "ses_a")
	if err != nil {
		t.Fatalf("list ses_a: %v", err)
	}
	runsA, err := runs.ListRuns(ctx, "ses_a")
	if err != nil {
		t.Fatalf("list ses_a runs: %v", err)
	}
	itemsB, err := store.List(ctx, "ses_b")
	if err != nil {
		t.Fatalf("list ses_b: %v", err)
	}
	runsB, err := runs.ListRuns(ctx, "ses_b")
	if err != nil {
		t.Fatalf("list ses_b runs: %v", err)
	}
	if len(itemsA) != 1 || itemsA[0].ID() != "item_shared" || len(runsA) != 1 || runsA[0].ID() != "run_shared" {
		t.Fatalf("original transcript changed: items=%+v runs=%+v", itemsA, runsA)
	}
	if len(itemsB) != 0 || len(runsB) != 0 {
		t.Fatalf("conflicting transcript was re-parented: items=%+v runs=%+v", itemsB, runsB)
	}
}

func TestTranscriptStoreReplaceItemUsesExactOptimisticSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewTranscriptStore(db)
	now := time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC)
	original := testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID:  "ses_a",
		RunID:      "run_1",
		ID:         "item_child",
		OccurredAt: now,
		FinishedAt: now,
		Status:     transcript.ItemIncomplete,
		Kind:       transcript.ToolCall,
		Tool:       &transcript.ToolInvocation{Name: "delegate_task", Arguments: tool.Arguments{}},
	})
	if appendItemErr := store.AppendItem(t.Context(), original); appendItemErr != nil {
		t.Fatalf("seed Item: %v", appendItemErr)
	}
	failure := tool.Failure{
		Kind:   tool.FailureChildRunCanceled,
		Detail: "stop delegated branch",
	}
	replacement, err := original.ClassifyAbandonedToolCall(failure)
	if err != nil {
		t.Fatalf("classify Item: %v", err)
	}
	if replaceItemErr := store.ReplaceItem(t.Context(), original, replacement); replaceItemErr != nil {
		t.Fatalf("ReplaceItem: %v", replaceItemErr)
	}
	stored, found, err := store.Item(t.Context(), original.ID())
	if err != nil || !found {
		t.Fatalf("Item after replacement found=%t err=%v", found, err)
	}
	storedFailure, failed := stored.Failure()
	if !failed || storedFailure.Kind != tool.FailureChildRunCanceled {
		t.Fatalf("replaced Item = %+v, want child_run_canceled", stored)
	}

	staleFailure := tool.Failure{
		Kind:   tool.FailureChildRunCanceled,
		Detail: "overwrite newer result",
	}
	staleReplacement, err := original.ClassifyAbandonedToolCall(staleFailure)
	if err != nil {
		t.Fatalf("classify stale Item: %v", err)
	}
	err = store.ReplaceItem(t.Context(), original, staleReplacement)
	if !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("stale ReplaceItem error = %v, want ErrIdentityConflict", err)
	}
	stored, found, err = store.Item(t.Context(), original.ID())
	storedFailure, failed = stored.Failure()
	if err != nil || !found || !failed || storedFailure.Detail != failure.Detail {
		t.Fatalf("Item after stale replacement = %+v found=%t err=%v", stored, found, err)
	}
}

func TestTranscriptStoreKeepsOffloadRelationshipsImmutableAndOneToOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewTranscriptStore(db)
	now := time.Now().UTC()
	preview := tool.StringResult("preview")
	original := testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID: "ses_a", RunID: "run_1", ID: "item_1", OccurredAt: now,
		FinishedAt: now, Status: transcript.ItemCompleted,
		Kind: transcript.ToolCall,
		Tool: &transcript.ToolInvocation{
			Name: "shell", Result: &preview, Offload: &resultoffload.Ref{ID: "BLOB234"},
		},
	})
	if appendItemErr := store.AppendItem(t.Context(), original); appendItemErr != nil {
		t.Fatalf("seed item: %v", appendItemErr)
	}

	changedSnapshot := original.Snapshot()
	otherPreview := tool.StringResult("other preview")
	changedSnapshot.Tool = &transcript.ToolInvocation{
		Name: "shell", Result: &otherPreview, Offload: &resultoffload.Ref{ID: "OTHER234"},
	}
	changed, err := transcript.RestoreItem(changedSnapshot)
	if err != nil {
		t.Fatalf("restore changed item: %v", err)
	}
	if appendItemErr := store.AppendItem(t.Context(), changed); !errors.Is(appendItemErr, transcript.ErrIdentityConflict) {
		t.Fatalf("replace offload error = %v, want ErrIdentityConflict", appendItemErr)
	}

	duplicateSnapshot := original.Snapshot()
	duplicateSnapshot.Identity.ItemID = "item_2"
	duplicate, err := transcript.RestoreItem(duplicateSnapshot)
	if err != nil {
		t.Fatalf("restore duplicate item: %v", err)
	}
	if err := store.AppendItem(t.Context(), duplicate); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("reuse offload error = %v, want ErrIdentityConflict", err)
	}
}

func TestRevisionTablesRejectNumbersOutsideTheExactEnvelope(t *testing.T) {
	t.Parallel()

	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, revision := range []uint64{0, exactint.Maximum + 1} {
		for name, statement := range map[string]string{
			"sessions": `INSERT INTO sessions(
				id, title, title_search, workspace_path, workspace_search, started_at, updated_at,
				provider, model, revision
			) VALUES ('ses_revision', '', '', '/work', '/work', 1, 1, 'provider', 'model', ?)`,
			"session_plans": `INSERT INTO session_plans(session_id, steps, revision, updated_at)
				VALUES ('ses_revision', '[]', ?, 1)`,
			"schedules": `INSERT INTO schedules(id, instructions, cron, created_at, revision)
				VALUES ('sch_revision', 'review', '@daily', 1, ?)`,
		} {
			t.Run(fmt.Sprintf("%s/%d", name, revision), func(t *testing.T) {
				if _, err := db.ExecContext(t.Context(), statement, revision); err == nil {
					t.Fatalf("%s accepted revision %d", name, revision)
				}
			})
		}
	}
}
