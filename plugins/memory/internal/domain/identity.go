package agentmemory

import (
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

const (
	ItemIDPrefix            = "mem_"
	ItemIDEntropyBytes      = 16
	itemIDHexCharacters     = ItemIDEntropyBytes * 2
	MaximumItemIDCharacters = len(ItemIDPrefix) + itemIDHexCharacters
)

var itemIDPattern = fmt.Sprintf(
	`^%s[0-9a-f]{%d}$`,
	regexp.QuoteMeta(ItemIDPrefix),
	itemIDHexCharacters,
)

var errInvalidItemID = errors.New("agentmemory: invalid Item identity")

// Memory and transcript Items have independent owners, so their identities cannot be interchanged.
type ItemID struct{ text string }

func NewItemID(entropy [ItemIDEntropyBytes]byte) ItemID {
	return ItemID{text: ItemIDPrefix + hex.EncodeToString(entropy[:])}
}

func ParseItemID(raw string) (ItemID, error) {
	digest, ok := strings.CutPrefix(raw, ItemIDPrefix)
	if !ok {
		return ItemID{}, fmt.Errorf("%w: identity is not framed with %q", errInvalidItemID, ItemIDPrefix)
	}
	if err := identity.ValidateLowercaseHex(digest, itemIDHexCharacters); err != nil {
		return ItemID{}, fmt.Errorf("%w: identity %w", errInvalidItemID, err)
	}
	return ItemID{text: raw}, nil
}

func (i ItemID) String() string { return i.text }

func (i ItemID) validate() error {
	if i.text == "" {
		return errInvalidItemID
	}
	return nil
}

func (i ItemID) MarshalText() ([]byte, error) {
	if err := i.validate(); err != nil {
		return nil, err
	}
	return []byte(i.text), nil
}
func (i *ItemID) UnmarshalText(text []byte) error {
	value, err := ParseItemID(string(text))
	if err == nil {
		*i = value
	}
	return err
}

// itemIDModel mirrors the pattern-constrained string an ItemID marshals to, so
// schema derivation describes the text wire shape the opaque struct hides.
type itemIDModel string

func (itemIDModel) JSONSchemaPattern() string { return itemIDPattern }

// JSONSchemaModel projects ItemID's text encoding as its string wire model, as
// Scope's schema derivation requires for a struct with custom encoding.
func (ItemID) JSONSchemaModel() any { return itemIDModel("") }
