// Package sessionidentity owns exact opaque Flame Runtime session identities.
package sessionidentity

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"
)

// MaximumCharacters is the CLI domain envelope for an opaque Runtime Session
// identity. The runtime adapter proves it remains synchronized with the public
// protocol contract.
const MaximumCharacters = 256

// ID is one exact opaque Runtime session identity. Consumers may store,
// compare, and project it, but must never trim or otherwise repair it.
type ID struct {
	value string
}

// Parse admits an identity received from the Runtime, a command, or durable
// CLI state without normalizing it.
func Parse(value string) (ID, error) {
	if !utf8.ValidString(value) {
		return ID{}, errors.New("session id is not valid UTF-8")
	}
	if value == "" {
		return ID{}, errors.New("session id is empty")
	}
	if characters := utf8.RuneCountInString(value); characters > MaximumCharacters {
		return ID{}, fmt.Errorf(
			"session id has %d characters, maximum is %d",
			characters,
			MaximumCharacters,
		)
	}
	for _, character := range value {
		if unicode.IsSpace(character) || !unicode.IsPrint(character) {
			return ID{}, errors.New("session id contains whitespace or a non-printing character")
		}
	}
	return ID{value: value}, nil
}

// String projects the exact opaque identity at an adapter boundary.
func (i ID) String() string { return i.value }

// Validate rejects the invalid zero value and any corrupted in-memory state.
func (i ID) Validate() error {
	_, err := Parse(i.value)
	return err
}
