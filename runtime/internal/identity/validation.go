package identity

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrIncompleteModelSelection    = errors.New("model selection: provider and model must be set together")
	ErrReasoningEffortWithoutModel = errors.New("model selection: reasoning effort requires provider and model")
	ErrProviderIdentity            = errors.New("model selection: invalid provider identity")
	ErrModelIdentity               = errors.New("model selection: invalid model identity")
	ErrReasoningEffortIdentity     = errors.New("model selection: invalid reasoning effort identity")
)

// ValidateResource enforces the shared opaque resource envelope without
// assigning meaning to an identity or normalizing caller material.
func ValidateResource(kind, value string, maximumCharacters int) error {
	if err := ValidateText(value, maximumCharacters); err != nil {
		return fmt.Errorf("%s identity %w", kind, err)
	}
	return nil
}

// ValidateText is what every exact identity must be, whatever it names: a
// non-empty, valid UTF-8 string within its envelope, carrying no whitespace or
// non-printing character. It reports the defect as a phrase, so each caller
// keeps its own way of failing — naming the kind it validated, or carrying the
// sentinel its callers branch on.
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

// ValidateURISafeASCII is the byte envelope shared by every identity that must
// survive a URI path segment unescaped: 1 to maximumBytes of unreserved ASCII.
// Like ValidateText it reports the defect as a phrase, so each caller names the
// kind it validated. A byte is a character here because the set is ASCII.
func ValidateURISafeASCII(value string, maximumBytes int) error {
	envelope := fmt.Errorf("must contain 1 to %d URI-safe ASCII bytes", maximumBytes)
	if len(value) == 0 || len(value) > maximumBytes {
		return envelope
	}
	for index := range len(value) {
		character := value[index]
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' || character == '.' || character == ':' {
			continue
		}
		return envelope
	}
	return nil
}

// ValidateLowercaseHex is the canonical spelling every hex-encoded identity
// shares: exactly this many lowercase hexadecimal digits. hex.DecodeString
// admits uppercase, so the case rule cannot be left to it. Like ValidateText it
// reports the defect as a phrase the caller names its kind with.
func ValidateLowercaseHex(value string, characters int) error {
	if len(value) == characters && value == strings.ToLower(value) {
		if _, err := hex.DecodeString(value); err == nil {
			return nil
		}
	}
	return fmt.Errorf("must contain exactly %d lowercase hexadecimal characters", characters)
}

// ValidateEventIdentity owns the whole replay-cursor identity: the EventPrefix a
// transport frames an application cursor with, inside the envelope
// MaximumEventCharacters sizes for exactly that framed shape.
func ValidateEventIdentity(value string) error {
	if err := ValidateResource("event", value, MaximumEventCharacters); err != nil {
		return err
	}
	if !strings.HasPrefix(value, EventPrefix) {
		return fmt.Errorf("event identity is not framed with %q", EventPrefix)
	}
	return nil
}

func ValidateProviderIdentity(value string) error {
	return validateModelIdentity(value, MaximumProviderCharacters, ErrProviderIdentity)
}

func ValidateModelIdentity(value string) error {
	return validateModelIdentity(value, MaximumModelCharacters, ErrModelIdentity)
}

func ValidateReasoningEffortIdentity(value string) error {
	return validateModelIdentity(value, MaximumReasoningEffortCharacters, ErrReasoningEffortIdentity)
}

func ValidateModelSelection(provider, model, reasoningEffort string) error {
	if (provider == "") != (model == "") {
		return ErrIncompleteModelSelection
	}
	if model == "" && reasoningEffort != "" {
		return ErrReasoningEffortWithoutModel
	}
	if model == "" {
		return nil
	}
	if err := ValidateProviderIdentity(provider); err != nil {
		return err
	}
	if err := ValidateModelIdentity(model); err != nil {
		return err
	}
	if reasoningEffort != "" {
		return ValidateReasoningEffortIdentity(reasoningEffort)
	}
	return nil
}

func validateModelIdentity(value string, maximumCharacters int, identityError error) error {
	if err := ValidateText(value, maximumCharacters); err != nil {
		return fmt.Errorf("%w: %v", identityError, err)
	}
	return nil
}
