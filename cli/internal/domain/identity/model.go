package identity

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"
)

var ErrIncompleteModelSelection = errors.New("provider and model must be selected together")

// These are CLI product limits, not transport constants. The Runtime adapter
// owns the test that proves this consumer policy remains aligned with the
// Runtime contract while the inner model stays independent of that adapter.
const (
	MaximumProviderCharacters        = 64
	MaximumModelCharacters           = 256
	MaximumReasoningEffortCharacters = 32
)

func ValidateProvider(value string) error {
	return validateModelIdentity("provider", value, MaximumProviderCharacters)
}

func ValidateModel(value string) error {
	return validateModelIdentity("model", value, MaximumModelCharacters)
}

func ValidateReasoningEffort(value string) error {
	return validateModelIdentity("reasoning effort", value, MaximumReasoningEffortCharacters)
}

// ValidateModelSelection validates the zero-or-complete model identity. Empty provider and
// model mean the surrounding Runtime default; effort can only qualify an exact
// pair.
func ValidateModelSelection(provider, model, reasoningEffort string) error {
	if (provider == "") != (model == "") {
		return ErrIncompleteModelSelection
	}
	if model == "" {
		if reasoningEffort != "" {
			return errors.New("reasoning effort requires provider and model")
		}
		return nil
	}
	if err := ValidateProvider(provider); err != nil {
		return err
	}
	if err := ValidateModel(model); err != nil {
		return err
	}
	if reasoningEffort != "" {
		return ValidateReasoningEffort(reasoningEffort)
	}
	return nil
}

func validateModelIdentity(label, value string, maximumCharacters int) error {
	if value == "" {
		return fmt.Errorf("%s identity is empty", label)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s identity is not valid UTF-8", label)
	}
	if characters := utf8.RuneCountInString(value); characters > maximumCharacters {
		return fmt.Errorf("%s identity exceeds %d characters", label, maximumCharacters)
	}
	for _, character := range value {
		if unicode.IsSpace(character) || !unicode.IsPrint(character) {
			return fmt.Errorf("%s identity contains whitespace or a non-printing character", label)
		}
	}
	return nil
}
