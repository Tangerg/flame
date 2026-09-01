package identity

import (
	"errors"
	"fmt"
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
	if value == "" {
		return fmt.Errorf("%s identity is empty", kind)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s identity is not valid UTF-8", kind)
	}
	if characters := utf8.RuneCountInString(value); characters > maximumCharacters {
		return fmt.Errorf("%s identity has %d characters, maximum is %d", kind, characters, maximumCharacters)
	}
	for _, character := range value {
		if unicode.IsSpace(character) || !unicode.IsPrint(character) {
			return fmt.Errorf("%s identity contains whitespace or a non-printing character", kind)
		}
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
	if value == "" {
		return fmt.Errorf("%w: empty", identityError)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%w: invalid UTF-8", identityError)
	}
	if characters := utf8.RuneCountInString(value); characters > maximumCharacters {
		return fmt.Errorf("%w: %d characters exceeds %d", identityError, characters, maximumCharacters)
	}
	for _, character := range value {
		if unicode.IsSpace(character) || !unicode.IsPrint(character) {
			return fmt.Errorf("%w: contains whitespace or a non-printing character", identityError)
		}
	}
	return nil
}
