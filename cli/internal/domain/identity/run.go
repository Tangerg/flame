package identity

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

const (
	// MaximumResourceCharacters is the CLI domain envelope for ordinary opaque Runtime
	// execution identities. The Runtime binding proves it remains synchronized
	// with the public protocol contract.
	MaximumResourceCharacters = 256
	// MaximumEventCharacters is the separate envelope for a replay Event
	// identity, whose opaque value may contain a complete bounded cursor.
	MaximumEventCharacters = 65_540
)

// ValidateRun admits one exact logical Run identity without normalizing it.
func ValidateRun(value string) error {
	return validateOpaque("run id", value, MaximumResourceCharacters)
}

// ValidateSegment admits one exact execution-segment generation identity
// without normalizing it.
func ValidateSegment(value string) error {
	return validateOpaque("segment id", value, MaximumResourceCharacters)
}

// ValidateItem admits one exact transcript or interrupt item identity without
// normalizing it.
func ValidateItem(value string) error {
	return validateOpaque("item id", value, MaximumResourceCharacters)
}

// ValidateEvent admits one exact replay token within a stream segment without
// normalizing it.
func ValidateEvent(value string) error {
	return validateOpaque("event id", value, MaximumEventCharacters)
}

func validateOpaque(name, value string, maximumCharacters int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s is not valid UTF-8", name)
	}
	if value == "" {
		return fmt.Errorf("%s is empty", name)
	}
	if characters := utf8.RuneCountInString(value); characters > maximumCharacters {
		return fmt.Errorf("%s has %d characters, maximum is %d", name, characters, maximumCharacters)
	}
	for _, character := range value {
		if unicode.IsSpace(character) || !unicode.IsPrint(character) {
			return fmt.Errorf("%s contains whitespace or a non-printing character", name)
		}
	}
	return nil
}
