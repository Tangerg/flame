package plugin

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/qimu/jsonvalue"
	"github.com/Tangerg/qimu/portable"
)

type BoundContribution struct {
	Provider     Dependency
	Contribution ContributionID
	Server       mcpserver.ServerName
	Export       string
	Contract     jsonvalue.Value
}

type ResolvedRequirement struct {
	Contribution ContributionID
	ID           ContributionID
	Contract     jsonvalue.Value
	Optional     bool
	Bindings     []BoundContribution
}

// RequiredBindings freezes the entry's declared collaborators for its lifetime.
// Selection changes can produce another value, never alter this one.
type RequiredBindings struct{ values []ResolvedRequirement }

func NewRequiredBindings(values []ResolvedRequirement) (*RequiredBindings, error) {
	owned := &RequiredBindings{values: slices.Clone(values)}
	seen := make(map[[2]ContributionID]bool, len(values))
	for i, value := range owned.values {
		if err := value.Contribution.Validate(); err != nil {
			return nil, err
		}
		if err := value.ID.Validate(); err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%s/%s", value.Contribution, value.ID)
		if seen[[2]ContributionID{value.Contribution, value.ID}] {
			return nil, fmt.Errorf("%w: duplicate resolved requirement %s", ErrInvalid, key)
		}
		seen[[2]ContributionID{value.Contribution, value.ID}] = true
		contract, err := portable.ParseCapability(value.Contract.JSON())
		if err != nil {
			return nil, fmt.Errorf("%w: requirement %s contract: %w", ErrInvalid, key, err)
		}
		if !value.Optional && len(value.Bindings) == 0 || contract.Descriptor().Composition == portable.Selection && len(value.Bindings) > 1 {
			return nil, fmt.Errorf("%w: requirement %s binding cardinality", ErrInvalid, key)
		}
		owned.values[i].Contract = contract.Identity()
		owned.values[i].Bindings = slices.Clone(value.Bindings)
		type provider struct {
			installation identity.InstallationID
			contribution ContributionID
		}
		providers := map[provider]bool{}
		for _, binding := range value.Bindings {
			if err := binding.Provider.Validate(); err != nil {
				return nil, err
			}
			if err := binding.Contribution.Validate(); err != nil {
				return nil, err
			}
			if err := binding.Server.Validate(); err != nil {
				return nil, err
			}
			if !portable.ValidExportName(binding.Export) || binding.Contract != contract.Identity() {
				return nil, fmt.Errorf("%w: requirement %s binding contract", ErrInvalid, key)
			}
			providerKey := provider{binding.Provider.InstallationID, binding.Contribution}
			if providers[providerKey] {
				return nil, fmt.Errorf("%w: requirement %s has a duplicate provider", ErrInvalid, key)
			}
			providers[providerKey] = true
		}
	}
	return owned, nil
}

func (b *RequiredBindings) Snapshot() []ResolvedRequirement {
	if b == nil {
		return nil
	}
	values := slices.Clone(b.values)
	for i := range values {
		values[i].Bindings = slices.Clone(values[i].Bindings)
	}
	return values
}

func (b *RequiredBindings) Allows(provider Dependency, contribution ContributionID, contract jsonvalue.Value) bool {
	if b == nil {
		return false
	}
	for _, value := range b.values {
		for _, binding := range value.Bindings {
			if binding.Provider == provider && binding.Contribution == contribution && binding.Contract == contract {
				return true
			}
		}
	}
	return false
}
