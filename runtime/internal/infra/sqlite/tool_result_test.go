package sqlite_test

import (
	"crypto/rand"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func newToolResultStore(t *testing.T) *sqlite.ToolResultStore {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewToolResultStore(db)
}

func stageShellResult(t *testing.T, store *sqlite.ToolResultStore, sessionID, body string) toolresult.ID {
	t.Helper()
	id := toolresult.ID(rand.Text())
	if err := store.Stage(t.Context(), toolresult.Stage{
		ID: id, SessionID: sessionID, Body: body,
	}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	return id
}

func TestToolResultStageRoundTrip(t *testing.T) {
	store := newToolResultStore(t)
	const (
		sessID = "sess-1"
		body   = "the full, oversized tool output that was offloaded"
	)
	id := stageShellResult(t, store, sessID, body)

	got, found, err := store.Fetch(t.Context(), sessID, id)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !found {
		t.Fatal("fetch reported the just-offloaded body as missing")
	}
	if got != body {
		t.Fatalf("fetched body = %q, want %q", got, body)
	}
}

func TestToolResultFetchIsSessionScoped(t *testing.T) {
	store := newToolResultStore(t)
	id := stageShellResult(t, store, "owner", "secret")
	// A different session must not read another session's offloaded body.
	if _, found, err := store.Fetch(t.Context(), "intruder", id); err != nil || found {
		t.Fatalf("cross-session fetch = (found %v, err %v), want (false, nil)", found, err)
	}
}

func TestToolResultFetchUnknownIDIsRecoverableMiss(t *testing.T) {
	store := newToolResultStore(t)
	if _, found, err := store.Fetch(t.Context(), "s", "DOESNOTEXIST"); err != nil || found {
		t.Fatalf("unknown id = (found %v, err %v), want (false, nil)", found, err)
	}
}

func TestToolResultDropSession(t *testing.T) {
	store := newToolResultStore(t)
	id := stageShellResult(t, store, "doomed", "body")
	if err := store.DropSession(t.Context(), "doomed"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, found, err := store.Fetch(t.Context(), "doomed", id); err != nil || found {
		t.Fatalf("post-drop fetch = (found %v, err %v), want (false, nil)", found, err)
	}
}

func TestToolResultDiscardAndStartupPurgeOnlyRemoveUnboundBlobs(t *testing.T) {
	transcriptStore, store := openTranscriptAndBlobs(t)
	discardedID := stageShellResult(t, store, "ses_1", "discard me")
	if err := store.Discard(t.Context(), "ses_1", toolresult.Ref{ID: discardedID}); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if _, found, err := store.Fetch(t.Context(), "ses_1", discardedID); err != nil || found {
		t.Fatalf("discarded fetch = (found %v, err %v), want (false, nil)", found, err)
	}

	boundID := stageShellResult(t, store, "ses_1", "keep me")
	boundRef := toolresult.Ref{ID: boundID}
	if err := transcriptStore.AppendItem(t.Context(), toolItem("ses_1", "item_1", "preview", &boundRef)); err != nil {
		t.Fatal(err)
	}
	if err := store.Discard(t.Context(), "ses_1", boundRef); err != nil {
		t.Fatalf("discard bound: %v", err)
	}
	if _, found, err := store.Fetch(t.Context(), "ses_1", boundID); err != nil || !found {
		t.Fatalf("bound fetch after discard = (found %v, err %v), want (true, nil)", found, err)
	}

	stageShellResult(t, store, "ses_1", "stale after crash")
	removed, err := store.PurgeUnbound(t.Context())
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if removed != 1 {
		t.Fatalf("purged = %d, want 1", removed)
	}
	if body, found, err := store.Fetch(t.Context(), "ses_1", boundID); err != nil || !found || body != "keep me" {
		t.Fatalf("bound fetch after purge = (%q, %v, %v)", body, found, err)
	}
}

func TestToolResultStoreRejectsIncompleteIdentity(t *testing.T) {
	store := newToolResultStore(t)
	valid := toolresult.Stage{ID: toolresult.ID("BLOB234"), SessionID: "ses_1", Body: "body"}
	missingSession := valid
	missingSession.SessionID = ""
	if err := store.Stage(t.Context(), missingSession); err == nil {
		t.Fatal("Stage accepted an empty session ID")
	}
	missingBody := valid
	missingBody.Body = ""
	if err := store.Stage(t.Context(), missingBody); err == nil {
		t.Fatal("Stage accepted an empty body")
	}
}

func TestToolResultItemBindingListAndRestore(t *testing.T) {
	transcriptStore, store := openTranscriptAndBlobs(t)
	unbound := stageShellResult(t, store, "source", "unbound body")
	id := stageShellResult(t, store, "source", "full body")
	ref := toolresult.Ref{ID: id}
	if err := transcriptStore.AppendItem(t.Context(), toolItem("source", "item_1", "preview", &ref)); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := transcriptStore.AppendItem(t.Context(), toolItem("source", "item_1", "preview", &ref)); err != nil {
		t.Fatalf("replayed bind: %v", err)
	}
	if err := transcriptStore.AppendItem(t.Context(), toolItem("source", "item_2", "preview", &ref)); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("conflicting bind = %v, want ErrIdentityConflict", err)
	}
	foreign := stageShellResult(t, store, "other", "foreign body")
	if err := transcriptStore.AppendItem(t.Context(), toolItem("source", "item_3", "preview", &toolresult.Ref{ID: foreign})); err == nil {
		t.Fatal("an Item offloaded to another Session's body")
	}

	blobs, err := store.List(t.Context(), "source")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(blobs) != 1 || blobs[0].ID == unbound || blobs[0].ID != id || blobs[0].Body != "full body" {
		t.Fatalf("listed blobs = %+v, want exact bound blob", blobs)
	}
	blob := blobs[0]
	if err := store.DropSession(t.Context(), "source"); err != nil {
		t.Fatal(err)
	}
	blob.SessionID = "restored"
	blob.CreatedAt = time.Unix(blob.CreatedAt.Unix(), 0).UTC()
	if err := store.Restore(t.Context(), blob); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got, found, err := store.Fetch(t.Context(), "restored", id); err != nil || !found || got != "full body" {
		t.Fatalf("restored fetch = (%q, %v, %v)", got, found, err)
	}
}

func TestToolResultIDsAreSessionScoped(t *testing.T) {
	store := newToolResultStore(t)
	id := stageShellResult(t, store, "owner", "owner body")
	copied := toolresult.Blob{ID: id, SessionID: "fork", Body: "fork body", CreatedAt: time.Now().UTC()}
	if err := store.Restore(t.Context(), copied); err != nil {
		t.Fatalf("restore under another Session: %v", err)
	}
	for session, want := range map[string]string{"owner": "owner body", "fork": "fork body"} {
		if body, found, err := store.Fetch(t.Context(), session, id); err != nil || !found || body != want {
			t.Fatalf("fetch %s = (%q, %v, %v), want %q", session, body, found, err, want)
		}
	}
	if err := store.Restore(t.Context(), copied); !errors.Is(err, toolresult.ErrIdentityConflict) {
		t.Fatalf("restore over a held id = %v, want ErrIdentityConflict", err)
	}
}
