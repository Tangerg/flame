package models

import (
	"sync/atomic"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

type RoleState struct {
	role atomic.Pointer[modelref.Role]
}

func NewRoleState(initial modelref.Role) *RoleState {
	state := &RoleState{}
	state.store(initial)
	return state
}

func (r *RoleState) Role() modelref.Role {
	if r == nil {
		return modelref.Role{}
	}
	role := r.role.Load()
	if role == nil {
		return modelref.Role{}
	}
	return *role
}

func (r *RoleState) store(role modelref.Role) {
	r.role.Store(&role)
}
