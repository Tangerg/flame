package sqlite_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestSessionStorePersistsExactReplacement(t *testing.T) {
	store := newTempDB(t)
	current := testsupport.MustRestoreSession(session.Snapshot{
		ID: "ses_1", Title: "Before", Workspace: testsupport.MustWorkspace("/work"),
		StartedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0), Revision: 1,
	})
	if err := store.Insert(t.Context(), current); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	title := "After"
	replacement, changed, err := current.Apply(
		session.Patch{Title: &title}, time.Unix(2, 0),
	)
	if err != nil || !changed {
		t.Fatalf("Apply changed=%v err=%v", changed, err)
	}
	change, err := session.NextReplacement(current, replacement)
	if err != nil {
		t.Fatalf("NextReplacement: %v", err)
	}
	if saveErr := store.Save(t.Context(), change); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}
	got, err := store.Get(t.Context(), current.ID())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Snapshot() != replacement.Snapshot() {
		t.Fatalf("saved = %+v, want %+v", got.Snapshot(), replacement.Snapshot())
	}
}

func TestSessionStoreRejectsStaleReplacement(t *testing.T) {
	store := newTempDB(t)
	current := testsupport.MustRestoreSession(session.Snapshot{ID: "ses_1", Workspace: testsupport.MustWorkspace("/work")})
	if err := store.Insert(t.Context(), current); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	title := "First"
	first, _, _ := current.Apply(session.Patch{Title: &title}, time.Unix(2, 0))
	firstChange, err := session.NextReplacement(current, first)
	if err != nil {
		t.Fatalf("prepare first replacement: %v", err)
	}
	if err := store.Save(t.Context(), firstChange); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	title = "Stale"
	stale, _, _ := current.Apply(session.Patch{Title: &title}, time.Unix(3, 0))
	staleChange, err := session.NextReplacement(current, stale)
	if err != nil {
		t.Fatalf("prepare stale replacement: %v", err)
	}
	if err := store.Save(t.Context(), staleChange); !errors.Is(err, session.ErrRevisionConflict) {
		t.Fatalf("stale Save error = %v, want ErrRevisionConflict", err)
	}
	got, err := store.Get(t.Context(), current.ID())
	if err != nil || got.Title() != "First" || got.Revision() != first.Revision() {
		t.Fatalf("after stale Save = %+v, err=%v", got.Snapshot(), err)
	}
}

func TestSessionStoreRejectsInvalidWriteShape(t *testing.T) {
	store := newTempDB(t)
	initial := testsupport.MustRestoreSession(session.Snapshot{ID: "ses_1", Workspace: testsupport.MustWorkspace("/work")})
	initialChange, err := session.InitialReplacement(initial)
	if err != nil {
		t.Fatalf("InitialReplacement: %v", err)
	}
	if err := store.Save(t.Context(), initialChange); !errors.Is(err, session.ErrInvalid) {
		t.Fatalf("initial Save error = %v, want ErrInvalid", err)
	}
	title := "Revision two"
	replacement, _, _ := initial.Apply(session.Patch{Title: &title}, time.Unix(2, 0))
	if err := store.Insert(t.Context(), replacement); !errors.Is(err, session.ErrInvalid) {
		t.Fatalf("non-initial Insert error = %v, want ErrInvalid", err)
	}
}
