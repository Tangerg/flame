package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func newUtilityRoleStore(t *testing.T) *sqlite.UtilityRoleStore {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewUtilityRoleStore(db)
}

func TestUtilityRoleStore_RoundTrip(t *testing.T) {
	s := newUtilityRoleStore(t)
	ctx := context.Background()

	// Unset → zero role, no error.
	if role, present, err := s.LoadUtilityRole(ctx); err != nil || present || role.Configured() {
		t.Fatalf("empty load = (%+v, %t, %v); want (zero, false, nil)", role, present, err)
	}

	// Save then load round-trips.
	if err := s.SaveUtilityRole(ctx, mustStoredRole(t, "anthropic", "claude-haiku-4-5")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if role, present, err := s.LoadUtilityRole(ctx); err != nil || !present || role.Provider() != "anthropic" || role.Model() != "claude-haiku-4-5" {
		t.Fatalf("load = (%+v, %t, %v); want (anthropic, claude-haiku-4-5, true, nil)", role, present, err)
	}

	// Save again upserts the single row (no duplicate, latest wins).
	if err := s.SaveUtilityRole(ctx, mustStoredRole(t, "openai", "gpt-5-mini")); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if role, _, _ := s.LoadUtilityRole(ctx); role.Provider() != "openai" || role.Model() != "gpt-5-mini" {
		t.Fatalf("load after re-save = %+v; want (openai, gpt-5-mini)", role)
	}

	// Clearing (zero role) round-trips as unset.
	if err := s.SaveUtilityRole(ctx, modelref.Role{}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if role, present, err := s.LoadUtilityRole(ctx); err != nil || !present || role.Configured() {
		t.Fatalf("load after clear = (%+v, %t, %v); want explicit empty role", role, present, err)
	}
}

func TestUtilityRoleStoreRejectsPartialPersistedSelection(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, execErr := db.Exec(`INSERT INTO utility_role (id, provider, model) VALUES (1, ?, ?)`, "anthropic", ""); execErr != nil {
		t.Fatalf("seed corrupt role: %v", execErr)
	}
	_, _, err = sqlite.NewUtilityRoleStore(db).LoadUtilityRole(context.Background())
	if !errors.Is(err, modelref.ErrIncomplete) {
		t.Fatalf("load partial role error = %v, want %v", err, modelref.ErrIncomplete)
	}
}

func mustStoredRole(t testing.TB, provider, model string) modelref.Role {
	t.Helper()
	role, err := modelref.NewRole(provider, model)
	if err != nil {
		t.Fatal(err)
	}
	return role
}
