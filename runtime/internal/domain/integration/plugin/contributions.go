package plugin

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/identity"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/jsoninput"
)

// ContributionID identifies a declaration within its package, independently of
// the process or export that implements it.
type ContributionID string

func ParseContributionID(value string) (ContributionID, error) {
	id := ContributionID(value)
	return id, id.Validate()
}
func (id ContributionID) String() string { return string(id) }
func (id ContributionID) Validate() error {
	if !identity.ValidDeclarationID(string(id)) {
		return fmt.Errorf("%w: contribution identity", ErrInvalid)
	}
	return nil
}

// Contribution is an admitted contract-to-export declaration. Composition and
// role semantics belong to Contract; they are not independently authored here.
type Contribution struct {
	Background bool
	ID         ContributionID
	Server     mcpserver.ServerName
	Export     string
	Contract   jsoninput.Input
	Requires   []Requirement
}

// Requirement names one collaborator of this contribution. An optional
// collection is composition data and does not impose an activation edge.
type Requirement struct {
	ID       ContributionID
	Contract jsoninput.Input
	Optional bool
}

func (v Contribution) clone() Contribution {
	v.Requires = slices.Clone(v.Requires)
	return v
}

func (b *Builder) AdmitContribution(value Contribution) error {
	value = value.clone()
	if err := value.ID.Validate(); err != nil {
		return err
	}
	if !capability.ValidExportName(value.Export) {
		return refuse(DiagnosticInvalidDeclaration, "invalid contribution export %q", value.Export)
	}
	contract, err := capability.Parse(value.Contract.JSON())
	if err != nil {
		return refuse(DiagnosticInvalidDeclaration, "invalid contribution contract: %v", err)
	}
	value.Contract = contract.Identity()
	seen := make(map[ContributionID]bool, len(value.Requires))
	for i, requirement := range value.Requires {
		if err := requirement.ID.Validate(); err != nil {
			return err
		}
		if seen[requirement.ID] {
			return refuse(DiagnosticInvalidDependencies, "duplicate dependency %q", requirement.ID)
		}
		seen[requirement.ID] = true
		required, err := capability.Parse(requirement.Contract.JSON())
		if err != nil {
			return refuse(DiagnosticInvalidDependencies, "invalid dependency %q contract: %v", requirement.ID, err)
		}
		value.Requires[i].Contract = required.Identity()
	}
	if slices.ContainsFunc(b.declaration.Contributions, func(current Contribution) bool { return current.ID == value.ID }) {
		return refuse(DiagnosticInvalidDeclaration, "duplicate contribution %q", value.ID)
	}
	if slices.ContainsFunc(b.declaration.PointValues, func(current PointValue) bool { return current.ID == value.ID }) {
		return refuse(DiagnosticInvalidDeclaration, "duplicate contribution %q", value.ID)
	}
	if slices.ContainsFunc(b.declaration.Contributions, func(current Contribution) bool {
		return current.Server == value.Server && current.Export == value.Export && current.Contract != value.Contract
	}) {
		return refuse(DiagnosticInvalidDeclaration, "contribution %q conflicts with the same server export", value.ID)
	}
	if _, found := b.release().server(value.Server); !found {
		return refuse(DiagnosticInvalidDependencies, "contribution %q requires a declared server", value.ID)
	}
	b.declaration.Contributions = append(b.declaration.Contributions, value)
	return nil
}

func (r Release) Contribution(id ContributionID) (Contribution, bool) {
	index := slices.IndexFunc(r.declaration.Contributions, func(value Contribution) bool { return value.ID == id })
	if index < 0 {
		return Contribution{}, false
	}
	return r.declaration.Contributions[index].clone(), true
}
func (r Release) HasImplementation(server mcpserver.ServerName, required *capability.Contract) bool {
	return required != nil && slices.ContainsFunc(r.declaration.Contributions, func(value Contribution) bool {
		return value.Server == server && value.Contract == required.Identity()
	})
}
