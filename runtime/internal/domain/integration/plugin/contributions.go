package plugin

import (
	"fmt"
	"slices"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
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
	ModelProviders []string
	Background     bool
	ID             ContributionID
	Server         mcpserver.ServerName
	Export         string
	Contract       framevalue.JSON
	Requires       []Requirement
}

// Requirement names one collaborator of this contribution. An optional
// collection is composition data and does not impose an activation edge.
type Requirement struct {
	ID       ContributionID
	Contract framevalue.JSON
	Optional bool
}

func (c Contribution) clone() Contribution {
	c.ModelProviders = slices.Clone(c.ModelProviders)
	c.Requires = slices.Clone(c.Requires)
	return c
}

func (b *Builder) AdmitContribution(value Contribution) error {
	value = value.clone()
	if err := value.ID.Validate(); err != nil {
		return err
	}
	if !plugincontract.ValidExportName(value.Export) {
		return refuse(DiagnosticInvalidDeclaration, "invalid contribution export %q", value.Export)
	}
	contract, err := plugincontract.ParseCapability(value.Contract.Bytes())
	if err != nil {
		return refuse(DiagnosticInvalidDeclaration, "invalid contribution contract: %v", err)
	}
	value.Contract = contract.Identity()
	if err := value.admitModelProviders(contract); err != nil {
		return refuse(DiagnosticInvalidDeclaration, "contribution %q: %v", value.ID, err)
	}
	seen := make(map[ContributionID]bool, len(value.Requires))
	for i, requirement := range value.Requires {
		if err := requirement.ID.Validate(); err != nil {
			return err
		}
		if seen[requirement.ID] {
			return refuse(DiagnosticInvalidDependencies, "duplicate dependency %q", requirement.ID)
		}
		seen[requirement.ID] = true
		required, err := plugincontract.ParseCapability(requirement.Contract.Bytes())
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
func (r Release) HasImplementation(server mcpserver.ServerName, required *plugincontract.Capability) bool {
	return required != nil && slices.ContainsFunc(r.declaration.Contributions, func(value Contribution) bool {
		return value.Server == server && value.Contract == required.Identity()
	})
}

func (c *Contribution) admitModelProviders(contract *plugincontract.Capability) error {
	descriptor := contract.Descriptor()
	if _, reserved := extension.ProviderFromRole(descriptor.Reference); reserved {
		return fmt.Errorf("model provider selection roles are reserved for the model adapter contract")
	}
	if descriptor.Reference != extension.ModelProviderContract {
		if len(c.ModelProviders) != 0 {
			return fmt.Errorf("model provider scopes require the model adapter contract")
		}
		return nil
	}
	if descriptor.Composition != plugincontract.Selection || len(c.ModelProviders) == 0 {
		return fmt.Errorf("model adapter requires explicit provider scopes")
	}
	slices.Sort(c.ModelProviders)
	for i, id := range c.ModelProviders {
		if err := modelchoice.ValidateProviderIdentity(id); err != nil {
			return err
		}
		if i > 0 && c.ModelProviders[i-1] == id {
			return fmt.Errorf("duplicate model provider scope %q", id)
		}
	}
	return nil
}

func (c Contribution) SelectionRoles() []string {
	if len(c.ModelProviders) == 0 {
		contract, err := plugincontract.ParseCapability(c.Contract.Bytes())
		if err != nil || contract.Descriptor().Composition != plugincontract.Selection {
			return nil
		}
		return []string{contract.Descriptor().Reference}
	}
	roles := make([]string, 0, len(c.ModelProviders))
	for _, provider := range c.ModelProviders {
		roles = append(roles, extension.ModelProviderRole(provider))
	}
	return roles
}
