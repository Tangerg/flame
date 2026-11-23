package plugin

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

type ProviderSelection struct{ record SelectionRecord }

type SelectionRecord struct {
	Role         string
	Installation identity.InstallationID
	Contribution ContributionID
}

func NewProviderSelection(record SelectionRecord) (ProviderSelection, error) {
	if record.Role == "" || utf8.RuneCountInString(record.Role) > MaximumRoleCharacters || !utf8.ValidString(record.Role) || strings.ContainsAny(record.Role, " \t\r\n") {
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
