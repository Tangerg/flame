package main

import (
	"database/sql"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
)

func TestDraftTransferDoesNotPublishHalfOfAMove(t *testing.T) {
	const source = "ses_transfer_source"
	const destination = "ses_transfer_destination"
	directory := t.TempDir()
	open := workbenchFactory(directory)
	openStore := func() *workbench.Store {
		t.Helper()
		store, err := open("")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		})
		return store
	}
	assertDraft := func(store *workbench.Store, id string, want authoring.Message) {
		t.Helper()
		if draft, _ := store.Draft(id); !draft.Equal(want) {
			t.Fatalf("session %s draft = %q, want %q", id, draft.Text, want.Text)
		}
	}
	store := openStore()
	moving := authoring.Message{Text: "edited source draft"}
	before := authoring.Message{Text: "destination draft"}
	remaining := authoring.Message{Text: "source baseline"}
	if err := store.SaveDraft(source, moving); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDraft(destination, before); err != nil {
		t.Fatal(err)
	}
	address := url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(directory, "authoring.db")), RawQuery: "mode=rw"}
	database, err := sql.Open("sqlite", address.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := database.Exec(`CREATE TRIGGER refuse_source_write
		BEFORE UPDATE ON authoring_records WHEN old.key = 'session/` + source + `'
		BEGIN SELECT RAISE(ABORT, 'source draft write refused'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.TransferSessionDraft(source, destination, remaining); err == nil {
		t.Fatal("transfer succeeded without updating its source")
	}
	assertDraft(store, source, moving)
	assertDraft(store, destination, before)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openStore()
	assertDraft(store, source, moving)
	assertDraft(store, destination, before)
	if _, err := database.Exec(`DROP TRIGGER refuse_source_write`); err != nil {
		t.Fatal(err)
	}
	if err := store.TransferSessionDraft(source, destination, remaining); err != nil {
		t.Fatal(err)
	}
	assertDraft(store, source, remaining)
	assertDraft(store, destination, authoring.Message{Text: before.Text + "\n\n" + moving.Text})
}
