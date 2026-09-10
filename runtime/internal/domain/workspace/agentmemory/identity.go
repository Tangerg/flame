package agentmemory

import (
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

const (
	ItemIDPrefix            = "mem_"
	ItemIDEntropyBytes      = 16
	itemIDHexCharacters     = ItemIDEntropyBytes * 2
	MaximumItemIDCharacters = len(ItemIDPrefix) + itemIDHexCharacters
)

var ItemIDPattern = fmt.Sprintf(
	`^%s[0-9a-f]{%d}$`,
	regexp.QuoteMeta(ItemIDPrefix),
	itemIDHexCharacters,
)

var ErrInvalidItemID = errors.New("agentmemory: invalid Item identity")

// ItemID is one exact durable Agent Memory handle. It is intentionally
// distinct from transcript Item identities: the two resources have unrelated
// owners, persistence and public operations despite both being called items.
type ItemID struct{ text string }

// NewItemID encodes a canonical memory identity from caller-supplied entropy.
// The persistence boundary owns the cryptographically random source.
func NewItemID(entropy [ItemIDEntropyBytes]byte) ItemID {
	return ItemID{text: ItemIDPrefix + hex.EncodeToString(entropy[:])}
}

// ParseItemID admits only the canonical spelling emitted by [NewItemID].
func ParseItemID(raw string) (ItemID, error) {
	digest, ok := strings.CutPrefix(raw, ItemIDPrefix)
	if !ok {
		return ItemID{}, fmt.Errorf("%w: identity is not framed with %q", ErrInvalidItemID, ItemIDPrefix)
	}
	if err := runtimeidentity.ValidateLowercaseHex(digest, itemIDHexCharacters); err != nil {
		return ItemID{}, fmt.Errorf("%w: identity %w", ErrInvalidItemID, err)
	}
	return ItemID{text: raw}, nil
}

func (i ItemID) String() string { return i.text }

// Validate reports whether the identity was minted or parsed. Its canonical
// spelling is established there, so an unconstructed item is all this can
// reject.
func (i ItemID) Validate() error {
	if i.text == "" {
		return ErrInvalidItemID
	}
	return nil
}
