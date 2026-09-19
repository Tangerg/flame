package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestExecutionTreeWriterFencingSurvivesRestart(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "trees.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewExecutorCheckpointStore(db)
	first := ExecutionTreeRecord{SessionID: "ses_tree", RootID: "root", Writer: "first", Digest: "cut1", Payload: []byte("first cut")}
	if err := store.SaveExecutionTree(ctx, "", "", first); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveExecutionTree(ctx, "", "", first); err != nil {
		t.Fatalf("repeat acknowledged head: %v", err)
	}
	next := first
	next.Writer, next.Digest, next.Payload = "second", "cut2", []byte("activated cut")
	if err := store.SaveExecutionTree(ctx, "first", "cut1", next); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = NewExecutorCheckpointStore(db)
	head, found, err := store.LoadExecutionTree(ctx, first.SessionID, first.RootID)
	if err != nil || !found || head.Writer != next.Writer || string(head.Payload) != string(next.Payload) {
		t.Fatalf("reopened head = %+v, %t, %v", head, found, err)
	}
	if err := store.SaveExecutionTree(ctx, "", "", first); !errors.Is(err, ErrExecutionTreeConflict) {
		t.Fatalf("historical retry = %v", err)
	}
	stale := next
	stale.Writer, stale.Digest = "first", "cut3"
	if err := store.SaveExecutionTree(ctx, "first", "cut1", stale); !errors.Is(err, ErrExecutionTreeConflict) {
		t.Fatalf("old writer = %v", err)
	}
	conflict := next
	conflict.Payload = []byte("different content")
	if err := store.SaveExecutionTree(ctx, "first", "cut1", conflict); !errors.Is(err, ErrExecutionTreeConflict) {
		t.Fatalf("same digest with different bytes = %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.SaveExecutionTree(canceled, next.Writer, next.Digest, stale); err == nil {
		t.Fatal("canceled write succeeded")
	}
	if err := store.DeleteSessionCheckpoints(ctx, first.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.LoadExecutionTree(ctx, first.SessionID, first.RootID); err != nil || found {
		t.Fatalf("deleted Session retained tree: %t, %v", found, err)
	}
}
