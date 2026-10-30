package persistence

import (
	"context"
	"database/sql"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

type utilityRoleStore struct {
	store *roleStore
}

func newUtilityRoleStore(db *sql.DB) *utilityRoleStore {
	return &utilityRoleStore{store: newRoleStore(db, "utility_role", "utility role")}
}

// A present zero role records an explicit choice of the Runtime default;
// absence permits first-run configuration seeding.
func (u *utilityRoleStore) LoadUtilityRole(ctx context.Context) (modelref.Role, bool, error) {
	return u.store.load(ctx)
}

func (u *utilityRoleStore) SaveUtilityRole(ctx context.Context, role modelref.Role) error {
	return u.store.save(ctx, role)
}
