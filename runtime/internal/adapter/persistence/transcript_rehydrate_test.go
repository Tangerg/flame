package persistence

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func openTranscriptAndBlobs(t *testing.T) (*TranscriptStore, *ToolResultStore) {
	t.Helper()
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return newTranscriptStore(databaseFor(db)), newToolResultStore(databaseFor(db))
}

func toolItem(sessionID, id, result string, ref *transcript.ToolResultRef) transcript.Item {
	value, err := tool.NewResult(result)
	if err != nil {
		panic(err)
	}
	at := time.Unix(1, 0).UTC()
	executionDuration := 500 * time.Millisecond
	return testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID:  sessionID,
		ID:         id,
		RunID:      "run-1",
		Kind:       transcript.ToolCall,
		Status:     transcript.ItemCompleted,
		OccurredAt: at,
		FinishedAt: at.Add(time.Second), ExecutionDuration: &executionDuration,
		Tool: &transcript.ToolInvocation{Name: "shell", Result: &value, Offload: ref},
	})
}

func TestTranscriptPaginationRejectsInvalidControls(t *testing.T) {
	store, _ := openTranscriptAndBlobs(t)

	if _, err := store.PageSessionItems(t.Context(), "sess-1", transcript.SequenceOrder("ascending"), 0, 1); err == nil {
		t.Fatal("PageSessionItems accepted an unknown order")
	}
	if _, err := store.PageSessionItems(t.Context(), "sess-1", transcript.OldestFirst, -1, 1); err == nil {
		t.Fatal("PageSessionItems accepted a negative sequence")
	}
	if _, err := store.PageSessionItems(t.Context(), "sess-1", transcript.OldestFirst, 0, -1); err == nil {
		t.Fatal("PageSessionItems accepted a negative limit")
	}
}

func TestTranscriptRehydratesOffloadedToolResult(t *testing.T) {
	tr, blobs := openTranscriptAndBlobs(t)
	const sess = "sess-1"
	full := strings.Repeat("Z", 300)

	id := stageShellResult(t, blobs, sess, full)
	preview := "offloaded preview"
	if len(preview) >= len(full) {
		t.Fatal("test setup: preview should be smaller than the full body")
	}
	ref := &transcript.ToolResultRef{ID: id}
	if err := tr.AppendItem(t.Context(), toolItem(sess, "item-1", preview, ref)); err != nil {
		t.Fatal(err)
	}

	items, err := tr.List(t.Context(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	invocation, present := testsupport.ToolInvocationOf(items[0])
	if !present {
		t.Fatal("rehydrated Item has no Tool invocation")
	}
	if got, ok := invocation.Result.String(); !ok || got != full {
		t.Fatalf("tool result not rehydrated: got %q, want the full %d-byte body", got, len(full))
	}
	duration, known := testsupport.ExecutionDurationOf(items[0])
	if !known || duration != 500*time.Millisecond {
		t.Fatalf("tool execution duration not rehydrated: got %v/%t", duration, known)
	}
}

func TestTranscriptModelOnlyOffloadKeepsStructuredResultAcrossAllReads(t *testing.T) {
	store, blobs := openTranscriptAndBlobs(t)
	const sessionID = "session-model-text"
	id := stageShellResult(t, blobs, sessionID, "full model-visible text")
	result, err := tool.ParseResult([]byte(`{"revision":7,"title":"report"}`))
	if err != nil {
		t.Fatal(err)
	}
	item := toolItem(sessionID, "model-text-item", "preview", &transcript.ToolResultRef{ID: id, ModelOnly: true}).Snapshot()
	call := item.Body.(transcript.ToolCallBody)
	call.Invocation.Result = &result
	item.Body = call
	original, err := transcript.RestoreItem(item)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendItem(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	for _, read := range []struct {
		name  string
		items func() ([]transcript.Item, error)
	}{
		{"stored", func() ([]transcript.Item, error) { return store.ListStored(t.Context(), sessionID) }},
		{"hydrated", func() ([]transcript.Item, error) { return store.List(t.Context(), sessionID) }},
		{"single", func() ([]transcript.Item, error) {
			value, found, err := store.Item(t.Context(), original.ID())
			if !found {
				return nil, err
			}
			return []transcript.Item{value}, err
		}},
	} {
		t.Run(read.name, func(t *testing.T) {
			items, err := read.items()
			if err != nil || len(items) != 1 {
				t.Fatalf("read = %d items, %v", len(items), err)
			}
			if !items[0].Equal(original) {
				t.Fatal("read changed the result or model-only reference")
			}
		})
	}
	if err := store.AppendItem(t.Context(), original); err != nil {
		t.Fatalf("idempotent write changed after hydration: %v", err)
	}
}

func TestTranscriptSurfacesMissingOffloadedToolResult(t *testing.T) {
	tr, blobs := openTranscriptAndBlobs(t)
	const sess = "sess-2"
	// A typed reference without its blob is durable corruption, not an ordinary
	// non-offloaded result, and must not be hidden as a harmless preview.
	if err := tr.AppendItem(t.Context(), toolItem(sess, "item-0", "preview", &transcript.ToolResultRef{ID: "GONE234BLOB"})); err == nil {
		t.Fatal("an Item offloaded to a body its Session does not hold")
	}
	id := stageShellResult(t, blobs, sess, "full body")
	if err := tr.AppendItem(t.Context(), toolItem(sess, "item-1", "missing offloaded preview", &transcript.ToolResultRef{ID: id})); err != nil {
		t.Fatal(err)
	}
	if err := blobs.DropSession(t.Context(), sess); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.List(t.Context(), sess); err == nil {
		t.Fatal("missing blob must surface a broken durable reference")
	}
}

func TestTranscriptLeavesOrdinaryToolResultUntouched(t *testing.T) {
	tr, _ := openTranscriptAndBlobs(t)
	const sess = "sess-3"
	const plain = "a normal, small tool result"
	if err := tr.AppendItem(t.Context(), toolItem(sess, "item-1", plain, nil)); err != nil {
		t.Fatal(err)
	}
	items, err := tr.List(t.Context(), sess)
	if err != nil {
		t.Fatal(err)
	}
	invocation, present := testsupport.ToolInvocationOf(items[0])
	if !present {
		t.Fatal("stored Item has no Tool invocation")
	}
	if got, _ := invocation.Result.String(); got != plain {
		t.Fatalf("ordinary result altered: %q", got)
	}
}

func TestDeleteRunDropsItsBoundToolResults(t *testing.T) {
	tr, blobs := openTranscriptAndBlobs(t)
	const sess = "sess-drop"
	id := stageShellResult(t, blobs, sess, "full body")
	ref := &transcript.ToolResultRef{ID: id}
	if err := tr.AppendItem(t.Context(), toolItem(sess, "item-1", "preview", ref)); err != nil {
		t.Fatal(err)
	}
	if err := tr.DeleteRun(t.Context(), sess, "run-1"); err != nil {
		t.Fatalf("DeleteRun: %v", err)
	}
	if _, found, err := blobs.Fetch(t.Context(), sess, id); err != nil || found {
		t.Fatalf("blob after DeleteRun = (found %v, err %v), want removed", found, err)
	}
}
