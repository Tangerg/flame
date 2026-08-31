package agentmemory

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	ItemIDPrefix            = "mem_"
	itemIDEntropyBytes      = 16
	itemIDHexCharacters     = itemIDEntropyBytes * 2
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

// NewItemID creates one cryptographically random canonical memory identity.
func NewItemID() (ItemID, error) {
	var entropy [itemIDEntropyBytes]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return ItemID{}, fmt.Errorf("%w: generate entropy: %w", ErrInvalidItemID, err)
	}
	return ItemID{text: ItemIDPrefix + hex.EncodeToString(entropy[:])}, nil
}

// ParseItemID admits only the canonical spelling emitted by [NewItemID].
func ParseItemID(raw string) (ItemID, error) {
	if len(raw) != MaximumItemIDCharacters || !strings.HasPrefix(raw, ItemIDPrefix) {
		return ItemID{}, fmt.Errorf("%w: expected %s followed by %d lowercase hexadecimal characters", ErrInvalidItemID, ItemIDPrefix, itemIDHexCharacters)
	}
	for _, character := range raw[len(ItemIDPrefix):] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return ItemID{}, fmt.Errorf("%w: identity is not canonical lowercase hexadecimal", ErrInvalidItemID)
		}
	}
	return ItemID{text: raw}, nil
}

func (i ItemID) String() string { return i.text }

func (i ItemID) Validate() error {
	_, err := ParseItemID(i.text)
	return err
}
