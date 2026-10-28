package persistence

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestSessionStorageCannotCreateCompetingFlagValues(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := newSessionStore(db)
	value := testsupport.MustRestoreSession(session.Snapshot{
		ID: "ses_flags", Workspace: testsupport.MustWorkspace("/work"), Selection: testsupport.DefaultModelSelection(),
		CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC(), Revision: 1,
	})
	if err := store.Insert(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE sessions SET favorite = 2 WHERE id = 'ses_flags'`,
		`UPDATE sessions SET isolated = -1 WHERE id = 'ses_flags'`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err == nil {
			t.Fatalf("storage accepted a non-boolean flag: %s", statement)
		}
	}
	got, err := store.Get(t.Context(), value.ID())
	if err != nil || got.Favorite() || got.Isolated() {
		t.Fatalf("rejected flag writes changed Session: %+v, %v", got.Snapshot(), err)
	}
}

func TestSessionStorageRejectsInvalidDecodedFlags(t *testing.T) {
	for _, column := range []string{"favorite", "isolated"} {
		t.Run(column, func(t *testing.T) {
			store := newTempDB(t)
			value := testsupport.MustRestoreSession(session.Snapshot{ID: "ses_flags", Workspace: testsupport.MustWorkspace("/work"), Favorite: true, Isolated: true})
			if err := store.Insert(t.Context(), value); err != nil {
				t.Fatal(err)
			}
			got, err := store.Get(t.Context(), value.ID())
			if err != nil || !got.Favorite() || !got.Isolated() {
				t.Fatalf("valid flags failed to round-trip: %+v, %v", got.Snapshot(), err)
			}
			if _, err := store.db.ExecContext(t.Context(), "PRAGMA ignore_check_constraints=ON"); err != nil {
				t.Fatal(err)
			}
			if _, err := store.db.ExecContext(t.Context(), "UPDATE sessions SET "+column+"=2 WHERE id=?", value.ID()); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Get(t.Context(), value.ID()); err == nil {
				t.Fatal("read normalized a corrupt flag into valid Session authority")
			}
			if _, err := store.List(t.Context()); err == nil {
				t.Fatal("list normalized a corrupt flag into valid Session authority")
			}
		})
	}
}
