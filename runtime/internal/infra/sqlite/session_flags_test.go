package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestSessionStorageCannotCreateCompetingFlagValues(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewSessionStore(db)
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
