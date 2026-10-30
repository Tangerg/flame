package modelref

import modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"

// Role is an optional provider/model assignment for a specialized Runtime
// capability. Unlike an execution model selection, it has no model-owned options;
// its zero value leaves the assignment unset for the consuming capability.
type Role struct {
	selection modelchoice.Selection
}

// NewRole constructs an optional specialized model assignment.
func NewRole(provider, model string) (Role, error) {
	selection, err := modelchoice.New(provider, model)
	if err != nil {
		return Role{}, err
	}
	return Role{selection: selection}, nil
}

// Configured reports whether r assigns an exact provider and model.
func (r Role) Configured() bool { return r.selection.Configured() }

// Provider returns the assigned provider, or "" when unset.
func (r Role) Provider() string { return r.selection.Provider() }

// Model returns the assigned model, or "" when unset.
func (r Role) Model() string { return r.selection.Model() }

// Selection projects a configured role into the exact execution selection
// consumed by model adapters; an unset role projects to the zero model selection.
func (r Role) Selection() modelchoice.Selection { return r.selection }

// String returns the complete role identity used in diagnostics.
func (r Role) String() string { return r.selection.String() }
