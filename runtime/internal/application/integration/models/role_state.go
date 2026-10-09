package models

import (
	"sync/atomic"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// RoleState projects the model role committed by Coordinator. Consumers can
// observe it; only the model configuration use case can publish a replacement.
type RoleState struct {
	role atomic.Pointer[modelref.Role]
}

// NewRoleState builds a live role assignment with initial as its current value.
func NewRoleState(initial modelref.Role) *RoleState {
	state := &RoleState{}
	state.store(initial)
	return state
}

// Role returns the current assignment. The zero value means no specialized
// model is configured.
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
