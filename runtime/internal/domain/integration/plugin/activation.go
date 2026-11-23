package plugin

import (
	"slices"

	framevalue "github.com/Tangerg/frame/value"
)

type Activation struct {
	Binding      framevalue.JSON
	Dependencies []Dependency
}

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
