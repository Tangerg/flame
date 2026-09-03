package models

import (
	"sync/atomic"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// RoleState owns one live model-role assignment. Its synchronization is kept
// inside the application boundary; consumers observe the immutable value through
// Role rather than sharing an atomic implementation detail.
type RoleState struct {
	role atomic.Pointer[modelref.Role]
}

// NewRoleState builds a live role assignment with initial as its current value.
func NewRoleState(initial modelref.Role) *RoleState {
	state := &RoleState{}
	state.Store(initial)
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

// Store atomically publishes the next immutable assignment.
func (r *RoleState) Store(role modelref.Role) {
	r.role.Store(&role)
}
