package plugin

import (
	"slices"

	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

// Activation is an immutable projection of a session-owned binding graph. Its
// identity includes configuration and ordered collaborators; releases alone cannot
// distinguish two different compositions inside the same installed bytes.
type Activation struct {
	Identity     fingerprint.Digest
	Dependencies []Dependency
}

func (a Activation) Clone() Activation { a.Dependencies = slices.Clone(a.Dependencies); return a }
func (a Activation) Validate() error {
	if err := a.Identity.Validate(); err != nil {
		return err
	}
	return ValidateDependencies(a.Dependencies)
}
func (a Activation) Equal(b Activation) bool {
	return a.Identity == b.Identity && slices.Equal(a.Dependencies, b.Dependencies)
}
