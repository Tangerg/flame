package plugin

import (
	"errors"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
	"github.com/Tangerg/flame/packages/go-sdk/resourceid"
)

type CompositionMember struct {
	Installation resourceid.InstallationID
	Contribution ContributionID
}

type CompositionRecord struct {
	Contract jsoninput.Input
	Members  []CompositionMember
}

// OrderedComposition owns consumer-selected outer-to-inner order. Releases are
// captured when a call binds, so editing this choice cannot retarget that call.
type OrderedComposition struct{ record CompositionRecord }

func NewOrderedComposition(record CompositionRecord) (OrderedComposition, error) {
	contract, err := capability.Parse(record.Contract.JSON())
	if err != nil {
		return OrderedComposition{}, errors.Join(ErrInvalid, err)
	}
	if contract.Descriptor().Composition != capability.Decoration {
		return OrderedComposition{}, errors.Join(ErrInvalid, errors.New("ordered composition requires a decoration contract"))
	}
	seen := make(map[CompositionMember]bool, len(record.Members))
	for _, member := range record.Members {
		if err := member.Installation.Validate(); err != nil {
			return OrderedComposition{}, err
		}
		if err := member.Contribution.Validate(); err != nil {
			return OrderedComposition{}, err
		}
		if seen[member] {
			return OrderedComposition{}, errors.Join(ErrInvalid, errors.New("duplicate composition member"))
		}
		seen[member] = true
	}
	record.Contract = contract.Identity()
	record.Members = slices.Clone(record.Members)
	if record.Members == nil {
		record.Members = []CompositionMember{}
	}
	return OrderedComposition{record: record}, nil
}

func (c OrderedComposition) Snapshot() CompositionRecord {
	record := c.record
	record.Members = slices.Clone(record.Members)
	return record
}

func (c OrderedComposition) Validate() error {
	_, err := NewOrderedComposition(c.record)
	return err
}
