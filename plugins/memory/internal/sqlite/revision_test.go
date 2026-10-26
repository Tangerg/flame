package sqlite

import (
	"errors"
	"testing"
	"time"

	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
)

func TestStoredMemoryPersistsRevisionAndFencesStaleIntent(t *testing.T) {
	store := newAgentMemoryStore(t)
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	original, _, err := store.Add(t.Context(), memory.ScopeUser, "", "original", now)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.Update(t.Context(), original.ID(), original.Revision(), new("corrected"), nil, now)
	if err != nil {
		t.Fatal(err)
	}
	store = NewAgentMemoryStore(store.db)
	if _, err := store.Update(t.Context(), original.ID(), original.Revision(), new("stale"), nil, now); !errors.Is(err, memory.ErrRevisionConflict) {
		t.Fatalf("stale edit: %v", err)
	}
	if err := store.Delete(t.Context(), original.ID(), original.Revision()); !errors.Is(err, memory.ErrRevisionConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	stored, found, err := store.get(t.Context(), original.ID())
	if err != nil || !found || stored.Revision() != current.Revision() || stored.Content() != current.Content() {
		t.Fatalf("stale intent changed persisted content: %+v, %v", stored, err)
	}
	if err := store.Delete(t.Context(), stored.ID(), stored.Revision()); err != nil {
		t.Fatal(err)
	}
}
