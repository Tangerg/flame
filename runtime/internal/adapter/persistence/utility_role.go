package persistence

import (
	"context"
	"database/sql"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// UtilityRoleStore persists the global utility-model role — the (provider,
// model) the in-house maintenance services (compaction / extraction / titling)
// run on — as a single row. The DB must have been opened via [Open] so the
// utility_role table exists.
type utilityRoleStore struct {
	store *roleStore
}

// NewUtilityRoleStore wires the given *sql.DB to the utility-role surface.
func newUtilityRoleStore(db *sql.DB) *utilityRoleStore {
	return &utilityRoleStore{store: newRoleStore(db, "utility_role", "utility role")}
}

// LoadUtilityRole returns the stored role and whether its row exists. A present
// zero role is an explicit choice to inherit the main Run model; absence permits
// first-run configuration seeding.
func (u *utilityRoleStore) LoadUtilityRole(ctx context.Context) (modelref.Role, bool, error) {
	return u.store.load(ctx)
}

// SaveUtilityRole upserts the single utility-role row. A zero role clears it
// back to the main Run model.
func (u *utilityRoleStore) SaveUtilityRole(ctx context.Context, role modelref.Role) error {
	return u.store.save(ctx, role)
}
