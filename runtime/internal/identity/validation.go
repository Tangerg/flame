package identity

import (
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
