package plugin

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	framevalue "github.com/Tangerg/frame/value"
)

type Dependency struct {
	InstallationID identity.InstallationID
	Digest         framevalue.Digest
}

func (d Dependency) Validate() error {
	if err := d.InstallationID.Validate(); err != nil {
		return fmt.Errorf("%w: dependency installation: %w", ErrInvalid, err)
	}
	if err := d.Digest.Validate(); err != nil {
		return fmt.Errorf("%w: dependency digest: %w", ErrInvalid, err)
	}
	return nil
}

func CompactDependencies(dependencies []Dependency) []Dependency {
	result := slices.Clone(dependencies)
	slices.SortFunc(result, func(left, right Dependency) int {
		return cmp.Or(cmp.Compare(left.InstallationID.String(), right.InstallationID.String()), cmp.Compare(left.Digest.String(), right.Digest.String()))
	})
	return slices.Compact(result)
}

func ValidateDependencies(dependencies []Dependency) error {
	for _, dependency := range dependencies {
		if err := dependency.Validate(); err != nil {
			return err
		}
	}
	if !slices.Equal(dependencies, CompactDependencies(dependencies)) {
		return fmt.Errorf("%w: dependencies are not canonical", ErrInvalid)
	}
	return nil
}
