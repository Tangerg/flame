package modelref

import "errors"

var errRoleReasoningEffort = errors.New("model role does not support reasoning effort")

// Role is an optional provider/model assignment for a specialized Runtime
// capability. Unlike an execution Selection, it has no model-owned options;
// its zero value leaves the capability unset or inheriting its main model.
type Role struct {
	selection Selection
}

// NewRole constructs an optional specialized model assignment.
func NewRole(provider, model string) (Role, error) {
	selection, err := New(provider, model)
	if err != nil {
		return Role{}, err
	}
	return Role{selection: selection}, nil
}

// Validate verifies that r contains only the provider/model pair its role
// stores and projects.
func (r Role) Validate() error {
	if err := r.selection.Validate(); err != nil {
		return err
	}
	if r.selection.ReasoningEffort() != "" {
		return errRoleReasoningEffort
	}
	return nil
}

// Configured reports whether r assigns an exact provider and model.
func (r Role) Configured() bool { return r.selection.Configured() }

// Provider returns the assigned provider, or "" when unset.
func (r Role) Provider() string { return r.selection.Provider() }

// Model returns the assigned model, or "" when unset.
func (r Role) Model() string { return r.selection.Model() }

// Selection projects a configured role into the exact execution selection
// consumed by model adapters; an unset role projects to the zero Selection.
func (r Role) Selection() Selection { return r.selection }

// String returns the complete role identity used in diagnostics.
func (r Role) String() string { return r.selection.String() }
