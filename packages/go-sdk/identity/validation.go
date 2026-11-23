package identity

import (
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

func ValidateResource(kind, value string, maximumCharacters int) error {
	if err := ValidateText(value, maximumCharacters); err != nil {
		return fmt.Errorf("%s identity %w", kind, err)
	}
	return nil
}

func ValidateText(value string, maximumCharacters int) error {
	if value == "" {
		return errors.New("is empty")
	}
	if !utf8.ValidString(value) {
		return errors.New("is not valid UTF-8")
	}
	if characters := utf8.RuneCountInString(value); characters > maximumCharacters {
		return fmt.Errorf("has %d characters, maximum is %d", characters, maximumCharacters)
	}
	for _, character := range value {
		if unicode.IsSpace(character) || !unicode.IsPrint(character) {
			return errors.New("contains whitespace or a non-printing character")
		}
	}
	return nil
}

const URISafeASCIIPattern = `^[A-Za-z0-9._:-]+$`

var uriSafeASCII = regexp.MustCompile(URISafeASCIIPattern)

func ValidateURISafeASCII(value string, maximumBytes int) error {
	envelope := fmt.Errorf("must contain 1 to %d URI-safe ASCII bytes", maximumBytes)
	if len(value) > maximumBytes || !uriSafeASCII.MatchString(value) {
		return envelope
	}
	return nil
}

// hex.DecodeString admits uppercase, so the case rule cannot be left to it.
func ValidateLowercaseHex(value string, characters int) error {
	if len(value) == characters && value == strings.ToLower(value) {
		if _, err := hex.DecodeString(value); err == nil {
			return nil
		}
	}
	return fmt.Errorf("must contain exactly %d lowercase hexadecimal characters", characters)
}

func ValidateEventIdentity(value string) error {
	if err := ValidateResource("event", value, MaximumEventCharacters); err != nil {
		return err
	}
	if !strings.HasPrefix(value, EventPrefix) {
		return fmt.Errorf("event identity is not framed with %q", EventPrefix)
	}
	return nil
}
