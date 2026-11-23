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
	Dependencies []Dependency
}

// Identity is the digest of the canonical binding document.
func (a Activation) Identity() framevalue.Digest { return a.Binding.Digest() }

func (a Activation) Clone() Activation { a.Dependencies = slices.Clone(a.Dependencies); return a }
func (a Activation) Validate() error {
	if err := a.Binding.Validate(); err != nil {
		return err
	}
	return ValidateDependencies(a.Dependencies)
}
func (a Activation) Equal(b Activation) bool {
	return a.Binding == b.Binding && slices.Equal(a.Dependencies, b.Dependencies)
}
