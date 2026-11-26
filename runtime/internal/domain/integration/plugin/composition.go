package plugin

import (
	"errors"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"

	framevalue "github.com/Tangerg/frame/value"
)

type CompositionMember struct {
	Installation identity.InstallationID
	Contribution ContributionID
}

type OrderedComposition struct {
	contract *plugincontract.Capability
	members  []CompositionMember
}

func NewOrderedComposition(encodedContract []byte, members []CompositionMember) (OrderedComposition, error) {
	contract, err := plugincontract.ParseCapability(encodedContract)
	if err != nil {
		return OrderedComposition{}, errors.Join(ErrInvalid, err)
	}
	if contract.Descriptor().Composition != plugincontract.Decoration {
		return OrderedComposition{}, errors.Join(ErrInvalid, errors.New("ordered composition requires a decoration contract"))
	}
	seen := make(map[CompositionMember]struct{}, len(members))
	for _, member := range members {
		if err := member.Installation.Validate(); err != nil {
			return OrderedComposition{}, err
		}
		if err := member.Contribution.Validate(); err != nil {
			return OrderedComposition{}, err
		}
		if _, found := seen[member]; found {
			return OrderedComposition{}, errors.Join(ErrInvalid, errors.New("duplicate composition member"))
		}
		seen[member] = struct{}{}
	}
	return OrderedComposition{contract: contract, members: slices.Clone(members)}, nil
}

func (c OrderedComposition) Valid() bool { return c.contract != nil }

func (c OrderedComposition) ContractReference() string {
	return c.contract.Descriptor().Reference
}

func (c OrderedComposition) ContractIdentity() framevalue.JSON { return c.contract.Identity() }

func (c OrderedComposition) Members() []CompositionMember { return slices.Clone(c.members) }
