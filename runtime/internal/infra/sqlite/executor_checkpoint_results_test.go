package sqlite_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestCheckpointOwnsResultsAcrossRestartAndReleasesThem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	results := sqlite.NewToolResultStore(db)
	checkpoints := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	retained := stageShellResult(t, results, "session-1", "retained body")
	orphan := stageShellResult(t, results, "session-1", "orphan body")
	checkpoint := storedExecutorCheckpoint("member_root", "session-1", `{"waiting":true}`)
	checkpoint.ToolResultIDs = []toolresult.ID{retained}
	if err := checkpoints.SaveCheckpoint(t.Context(), checkpoint); err != nil {
		t.Fatal(err)
	}
	if err := results.Discard(t.Context(), "session-1", toolresult.Ref{ID: retained}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	results = sqlite.NewToolResultStore(db)
	checkpoints = persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	if removed, err := results.PurgeUnbound(t.Context()); err != nil || removed != 1 {
		t.Fatalf("purge removed=%d err=%v", removed, err)
	}
	loaded, err := checkpoints.LoadCheckpoint(t.Context(), checkpoint.RootMemberID)
	if err != nil || !reflect.DeepEqual(loaded, checkpoint) {
		t.Fatalf("checkpoint changed: %+v %v", loaded, err)
	}
	if body, found, err := results.Fetch(t.Context(), "session-1", retained); err != nil || !found || body != "retained body" {
		t.Fatalf("retained result: %q %t %v", body, found, err)
	}
	if _, found, err := results.Fetch(t.Context(), "session-1", orphan); err != nil || found {
		t.Fatalf("orphan remains: %t %v", found, err)
	}
	// Once published, the Item owns the body even after its checkpoint is consumed.
	if err := results.Bind(t.Context(), "session-1", "item_result", "preview", toolresult.Ref{ID: retained}); err != nil {
		t.Fatal(err)
	}
	if err := checkpoints.DeleteCheckpoints(t.Context(), "session-1", []string{checkpoint.RootMemberID}); err != nil {
		t.Fatal(err)
	}
	if removed, err := results.PurgeUnbound(t.Context()); err != nil || removed != 0 {
		t.Fatalf("published result removed=%d err=%v", removed, err)
	}
}

func TestCheckpointResultReplacementIsAtomicAndSessionScoped(t *testing.T) {
	db, checkpoints := newExecutorCheckpointStorage(t)
	results := sqlite.NewToolResultStore(db)
	owned := stageShellResult(t, results, "session-1", "owned")
	foreign := stageShellResult(t, results, "session-2", "foreign")
	checkpoint := storedExecutorCheckpoint("member_root", "session-1", `{"waiting":true}`)
	checkpoint.ToolResultIDs = []toolresult.ID{owned}
	if err := checkpoints.SaveCheckpoint(t.Context(), checkpoint); err != nil {
		t.Fatal(err)
	}
	for _, id := range []toolresult.ID{foreign, "MISSING"} {
		replacement := checkpoint.Clone()
		replacement.Payload = []byte(`{"replacement":true}`)
		replacement.ToolResultIDs = []toolresult.ID{id}
		if err := checkpoints.SaveCheckpoint(t.Context(), replacement); err == nil {
			t.Fatal("invalid ownership accepted")
		}
		loaded, err := checkpoints.LoadCheckpoint(t.Context(), checkpoint.RootMemberID)
		if err != nil || !reflect.DeepEqual(loaded, checkpoint) {
			t.Fatalf("failed save changed checkpoint: %+v %v", loaded, err)
		}
	}
	replacement := checkpoint.Clone()
	replacement.ToolResultIDs = nil
	if err := checkpoints.SaveCheckpoint(t.Context(), replacement); err != nil {
		t.Fatal(err)
	}
	if _, err := results.PurgeUnbound(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, found, err := results.Fetch(t.Context(), "session-1", owned); err != nil || found {
		t.Fatalf("released reference remains: %t %v", found, err)
	}
}

func TestDeletingCheckpointReleasesUnpublishedResults(t *testing.T) {
	for _, mode := range []string{"consumed", "session deleted", "orphan checkpoint"} {
		t.Run(mode, func(t *testing.T) {
			db, checkpoints := newExecutorCheckpointStorage(t)
			results := sqlite.NewToolResultStore(db)
			id := stageShellResult(t, results, "session-1", "pending body")
			checkpoint := storedExecutorCheckpoint("member_root", "session-1", `{"waiting":true}`)
			checkpoint.ToolResultIDs = []toolresult.ID{id}
			if err := checkpoints.SaveCheckpoint(t.Context(), checkpoint); err != nil {
				t.Fatal(err)
			}
			var err error
			switch mode {
			case "consumed":
				err = checkpoints.DeleteCheckpoints(t.Context(), "session-1", []string{checkpoint.RootMemberID})
			case "session deleted":
				err = checkpoints.DeleteSessionCheckpoints(t.Context(), "session-1")
			case "orphan checkpoint":
				err = checkpoints.DeleteUnownedCheckpoints(t.Context(), nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			if removed, err := results.PurgeUnbound(t.Context()); err != nil || removed != 1 {
				t.Fatalf("released body remains: removed=%d err=%v", removed, err)
			}
		})
	}
}
