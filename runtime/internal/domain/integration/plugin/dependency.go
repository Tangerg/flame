package plugin

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

// Dependency names the exact release an execution admitted from one
// installation. Execution owns which dependencies it holds; the installation
// owner only asks whether any execution holds one before an unsafe change.
type Dependency struct {
	InstallationID resourceid.InstallationID
	Digest         fingerprint.Digest
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

// CompactDependencies returns the canonical set: sorted, without duplicates.
// Equal sets must encode identically wherever they bind a deployment identity.
func CompactDependencies(dependencies []Dependency) []Dependency {
	result := slices.Clone(dependencies)
	slices.SortFunc(result, func(left, right Dependency) int {
		return cmp.Or(cmp.Compare(left.InstallationID.String(), right.InstallationID.String()), cmp.Compare(left.Digest.String(), right.Digest.String()))
	})
	return slices.Compact(result)
}

// ValidateDependencies requires a valid canonical set, so a stored projection
// has exactly one spelling.
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

// RetainedReleases is every release that must stay admitted: the selected and
// staged release of each installation and each release an execution holds.
// Every other admitted release is unreferenced, and nothing can name it again
// without admitting its bytes anew, so its owner may reclaim it.
func RetainedReleases(installations []*Installation, held []Dependency) []fingerprint.Digest {
	var retained []fingerprint.Digest
	for _, installation := range installations {
		retained = append(retained, installation.record.Selected)
		if installation.record.Staged != nil {
			retained = append(retained, *installation.record.Staged)
		}
	}
	for _, dependency := range held {
		retained = append(retained, dependency.Digest)
	}
	slices.SortFunc(retained, func(left, right fingerprint.Digest) int { return cmp.Compare(left.String(), right.String()) })
	return slices.Compact(retained)
}
