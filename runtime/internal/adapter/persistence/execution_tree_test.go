package persistence

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestExecutionTreeWriterFencingSurvivesRestart(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "trees.db")
	db, err := openDatabase(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store := newExecutorCheckpointStore(db)
	first := executionTreeRecord{Sequence: 1, CommitID: "start", CommitDigest: "start", SessionID: "ses_tree", RootID: "root", Writer: "first", Digest: "cut1", Payload: []byte("first cut")}
	if err := store.saveExecutionTreeRecord(ctx, "", "", first); err != nil {
		t.Fatal(err)
	}
	if err := store.saveExecutionTreeRecord(ctx, "", "", first); err != nil {
		t.Fatalf("repeat acknowledged head: %v", err)
	}
	next := first
	next.Sequence, next.CommitID, next.CommitDigest = 0, "activate", "activate"
	next.Writer, next.Digest, next.Payload = "second", "cut2", []byte("activated cut")
	if err := store.saveExecutionTreeRecord(ctx, "first", "cut1", next); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = openDatabase(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = newExecutorCheckpointStore(db)
	head, found, err := store.loadExecutionTreeRecord(ctx, first.SessionID, first.RootID)
	if err != nil || !found || head.Writer != next.Writer || string(head.Payload) != string(next.Payload) {
		t.Fatalf("reopened head = %+v, %t, %v", head, found, err)
	}
	if err := store.saveExecutionTreeRecord(ctx, "", "", first); !errors.Is(err, errExecutionTreeConflict) {
		t.Fatalf("historical retry = %v", err)
	}
	stale := next
	stale.Writer, stale.Digest = "first", "cut3"
	if err := store.saveExecutionTreeRecord(ctx, "first", "cut1", stale); !errors.Is(err, errExecutionTreeConflict) {
		t.Fatalf("old writer = %v", err)
	}
	conflict := next
	conflict.Payload = []byte("different content")
	if err := store.saveExecutionTreeRecord(ctx, "first", "cut1", conflict); !errors.Is(err, errExecutionTreeConflict) {
		t.Fatalf("same digest with different bytes = %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.saveExecutionTreeRecord(canceled, next.Writer, next.Digest, stale); err == nil {
		t.Fatal("canceled write succeeded")
	}
	if err := store.DeleteSessionCheckpoints(ctx, first.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.loadExecutionTreeRecord(ctx, first.SessionID, first.RootID); err != nil || found {
		t.Fatalf("deleted Session retained tree: %t, %v", found, err)
	}
}

func TestExecutionTreeCommitIdentityRejectsContentCycles(t *testing.T) {
	ctx := t.Context()
	db, err := openDatabase(ctx, filepath.Join(t.TempDir(), "cycles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := newExecutorCheckpointStore(db)
	first := executionTreeRecord{SessionID: "session", RootID: "root", Writer: "writer", Sequence: 1, CommitID: "checkpoint:1", CommitDigest: "commit:1", Digest: "waiting", Payload: []byte("waiting")}
	if err := store.saveExecutionTreeRecord(ctx, "", "", first); err != nil {
		t.Fatal(err)
	}
	paused := first
	paused.Sequence, paused.CommitID, paused.CommitDigest, paused.Digest, paused.Payload = 2, "checkpoint:2", "commit:2", "paused", []byte("paused")
	if err := store.saveExecutionTreeRecord(ctx, first.Writer, first.Digest, paused); err != nil {
		t.Fatal(err)
	}
	resumed := first
	resumed.Sequence, resumed.CommitID, resumed.CommitDigest = 3, "checkpoint:3", "commit:3"
	if err := store.saveExecutionTreeRecord(ctx, paused.Writer, paused.Digest, resumed); err != nil {
		t.Fatal(err)
	}
	if err := store.saveExecutionTreeRecord(ctx, "", "", first); !errors.Is(err, errExecutionTreeConflict) {
		t.Fatalf("historical identical content accepted: %v", err)
	}
	if err := store.saveExecutionTreeRecord(ctx, paused.Writer, paused.Digest, resumed); err != nil {
		t.Fatalf("current retry rejected: %v", err)
	}
	for _, sequence := range []uint64{2, 3, 5} {
		invalid := resumed
		invalid.Sequence, invalid.CommitID = sequence, "new-checkpoint"
		if err := store.saveExecutionTreeRecord(ctx, resumed.Writer, resumed.Digest, invalid); !errors.Is(err, errExecutionTreeConflict) {
			t.Fatalf("sequence %d accepted: %v", sequence, err)
		}
	}
}

func TestExecutionTreeEffectIdentitySurvivesActivation(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "effects.db")
	db, err := openDatabase(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store := newExecutorCheckpointStore(db)
	head := executionTreeRecord{SessionID: "session", RootID: "root", Writer: "writer", Sequence: 1, CommitID: "start", CommitDigest: "start", Digest: "start", Payload: []byte("start")}
	if err := store.saveExecutionTreeRecord(ctx, "", "", head); err != nil {
		t.Fatal(err)
	}
	effect := head
	effect.Sequence, effect.CommitID, effect.CommitDigest, effect.Digest = 2, "effect:pending", "pending", "pending"
	if err := store.saveExecutionTreeRecord(ctx, head.Writer, head.Digest, effect); err != nil {
		t.Fatal(err)
	}
	activated := effect
	activated.Writer, activated.Sequence, activated.CommitID, activated.CommitDigest, activated.Digest = "new-writer", 0, "activation:new-writer", "activation", "activated"
	if err := store.saveExecutionTreeRecord(ctx, effect.Writer, effect.Digest, activated); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = openDatabase(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = newExecutorCheckpointStore(db)
	replay := effect
	replay.Writer, replay.Sequence = activated.Writer, 1
	if err := store.saveExecutionTreeRecord(ctx, activated.Writer, activated.Digest, replay); !errors.Is(err, errExecutionTreeConflict) {
		t.Fatalf("effect re-admitted by new writer: %v", err)
	}
	if err := store.DeleteSessionCheckpoints(ctx, head.SessionID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM execution_tree_commits`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("retained commit facts: %d, %v", count, err)
	}
}

func TestOpenRejectsExecutionTreesWithoutCommitIdentity(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := openDatabase(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`DROP TABLE execution_tree_commits`,
		`DROP TABLE execution_trees`,
		`CREATE TABLE execution_trees (root_id TEXT PRIMARY KEY, session_id TEXT NOT NULL, writer TEXT NOT NULL, digest TEXT NOT NULL, payload BLOB NOT NULL)`,
		`INSERT INTO execution_trees VALUES ('root', 'session', 'writer', 'digest', 'retained snapshot')`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := openDatabase(ctx, path); err == nil {
		reopened.Close()
		t.Fatal("old execution schema accepted without commit identity")
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var payload string
	if err := raw.QueryRowContext(ctx, `SELECT payload FROM execution_trees WHERE root_id = 'root'`).Scan(&payload); err != nil || payload != "retained snapshot" {
		t.Fatalf("rejected open modified existing state: %q, %v", payload, err)
	}
}
