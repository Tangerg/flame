package plugin

import (
	"slices"

	framevalue "github.com/Tangerg/frame/value"
)

// Activation is an immutable projection of a session-owned binding graph. Its
// identity includes configuration and ordered collaborators; releases alone cannot
// distinguish two different compositions inside the same installed bytes.
type Activation struct {
	Binding      framevalue.JSON
	Identity     framevalue.Digest
	Dependencies []Dependency
}

func (a Activation) Clone() Activation { a.Dependencies = slices.Clone(a.Dependencies); return a }
func (a Activation) Validate() error {
	if err := a.Binding.Validate(); err != nil {
		return err
	}
	if err := a.Identity.Validate(); err != nil {
		return err
	}
	return ValidateDependencies(a.Dependencies)
}
func (a Activation) Equal(b Activation) bool {
	return a.Binding == b.Binding && a.Identity == b.Identity && slices.Equal(a.Dependencies, b.Dependencies)
}
