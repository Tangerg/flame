package models

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/samber/lo"
)

type roleStore interface {
	Load(context.Context) (modelref.Role, bool, error)
	Save(context.Context, modelref.Role) error
}

type RoleSetting struct {
	store roleStore
	mu    sync.Mutex
	role  atomic.Pointer[modelref.Role]
}

func LoadRoleSetting(ctx context.Context, store roleStore) (*RoleSetting, error) {
	if lo.IsNil(store) {
		return nil, errors.New("models: role store is required")
	}
	role, _, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}
	setting := &RoleSetting{store: store}
	setting.role.Store(&role)
	return setting, nil
}

func (s *RoleSetting) Role() modelref.Role { return *s.role.Load() }

func (s *RoleSetting) set(
	ctx context.Context,
	role modelref.Role,
	validate func(context.Context, modelref.Role) error,
	complete operation.Reply[modelref.Role],
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if role.Configured() {
		if err := validate(ctx, role); err != nil {
			return err
		}
	}
	if err := complete(ctx, func(tx context.Context) (modelref.Role, error) {
		return role, s.store.Save(tx, role)
	}); err != nil {
		return err
	}
	s.role.Store(&role)
	return nil
}
