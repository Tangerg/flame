// Package modelidentity owns CLI-side restoration and command admission for
// Runtime model identities. The Runtime remains authoritative; these checks
// keep embedded/custom transports and persisted CLI state from bypassing the
// same public resource envelope.
package modelidentity

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"
)

var ErrIncompleteSelection = errors.New("provider and model must be selected together")

// These are CLI product limits, not transport constants. The Runtime adapter
// owns the test that proves this consumer policy remains aligned with the
// Runtime contract while the inner model stays independent of that adapter.
const (
	MaximumProviderCharacters        = 64
	MaximumModelCharacters           = 256
	MaximumReasoningEffortCharacters = 32
)

func Provider(value string) error {
	return validate("provider", value, MaximumProviderCharacters)
}

func Model(value string) error {
	return validate("model", value, MaximumModelCharacters)
}

func ReasoningEffort(value string) error {
	return validate("reasoning effort", value, MaximumReasoningEffortCharacters)
}

// Selection validates the zero-or-complete model identity. Empty provider and
// model mean the surrounding Runtime default; effort can only qualify an exact
// pair.
func Selection(provider, model, reasoningEffort string) error {
	if (provider == "") != (model == "") {
		return ErrIncompleteSelection
	}
	if model == "" {
		if reasoningEffort != "" {
			return errors.New("reasoning effort requires provider and model")
		}
		return nil
	}
	if err := Provider(provider); err != nil {
		return err
	}
	if err := Model(model); err != nil {
		return err
	}
	if reasoningEffort != "" {
		return ReasoningEffort(reasoningEffort)
	}
	return nil
}

func validate(label, value string, maximumCharacters int) error {
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
