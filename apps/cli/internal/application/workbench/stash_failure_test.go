package workbench

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/filesystem/statefile"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
)

type stashRollbackFailure struct {
	Persistence
	draftName   string
	stashWrites int
	enabled     bool
}

func (p *stashRollbackFailure) Replace(name string, body []byte) error {
	if p.enabled && name == stashesName {
		p.stashWrites++
		if p.stashWrites > 1 {
			return errors.New("injected stash rollback failure")
		}
	}
	return p.Persistence.Replace(name, body)
}

func (p *stashRollbackFailure) Remove(name string) error {
	if p.enabled && name == p.draftName {
		return errors.New("injected draft retirement failure")
	}
	return p.Persistence.Remove(name)
}

func (p *stashRollbackFailure) Close() error { return closePersistence(p.Persistence) }

func TestFailedStashRollbackPreservesRecoveryBeforeFurtherAuthoring(t *testing.T) {
	directory := t.TempDir()
	storage, err := statefile.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	failing := &stashRollbackFailure{Persistence: storage}
	store, err := Open(failing, Config{Random: bytes.NewBufferString("12345678")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	const sessionID = "session"
	draft := authoring.Message{Text: "retain the original transfer"}
	if err := store.SaveDraft(sessionID, draft); err != nil {
		t.Fatal(err)
	}
	failing.draftName = store.sessionStateName(sessionID)
	failing.enabled = true
	if _, err := store.StashDraft(sessionID, draft); err == nil || !strings.Contains(err.Error(), "requires reopen") {
		t.Fatalf("incomplete transfer was not fenced: %v", err)
	}
	failing.enabled = false
	if err := store.SaveDraft(sessionID, authoring.Message{Text: "replacement draft"}); err == nil {
		t.Fatal("new authoring overwrote the unresolved transfer")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if _, found := reopened.Draft(sessionID); found {
		t.Fatal("recovery retained the transferred draft")
	}
	stashes := reopened.Stashes()
	if len(stashes) != 1 || !stashes[0].Message.Equal(draft) {
		t.Fatalf("recovered stashes = %+v", stashes)
	}
	if err := reopened.SaveDraft(sessionID, authoring.Message{Text: "new authoring"}); err != nil {
		t.Fatal(err)
	}
}

func TestClosedMemoryWorkbenchRejectsNewAuthoring(t *testing.T) {
	store, err := OpenMemory(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Remember(authoring.Message{Text: "after close"}); err == nil {
		t.Fatal("closed workbench accepted history")
	}
	if err := store.SaveDraft("session", authoring.Message{Text: "after close"}); err == nil {
		t.Fatal("closed workbench accepted a draft")
	}
	if len(store.History()) != 0 {
		t.Fatal("closed workbench changed history")
	}
}
