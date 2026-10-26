package plugin

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

// ProviderSelection owns a product composition choice. Availability and a
// call's concrete release are separate projections; changing this choice cannot
// alter an already captured command route or transfer private business data.
type ProviderSelection struct{ record SelectionRecord }

type SelectionRecord struct {
	Role         string
	Installation identity.InstallationID
	Contribution ContributionID
}

func NewProviderSelection(record SelectionRecord) (ProviderSelection, error) {
	if record.Role == "" || len(record.Role) > 256 || !utf8.ValidString(record.Role) || strings.ContainsAny(record.Role, " \t\r\n") {
		return ProviderSelection{}, fmt.Errorf("%w: provider role reference", ErrInvalid)
	}
	if err := record.Installation.Validate(); err != nil {
		return ProviderSelection{}, err
	}
	if err := record.Contribution.Validate(); err != nil {
		return ProviderSelection{}, err
	}
	return ProviderSelection{record: record}, nil
}

func (s ProviderSelection) Snapshot() SelectionRecord { return s.record }
func (s ProviderSelection) Validate() error {
	_, err := NewProviderSelection(s.record)
	return err
}
