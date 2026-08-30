package modelref

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/modelidentity"
)

const (
	MaximumProviderIdentityCharacters = modelidentity.MaximumProviderCharacters
	MaximumModelIdentityCharacters    = modelidentity.MaximumModelCharacters
	MaximumReasoningEffortCharacters  = modelidentity.MaximumReasoningEffortCharacters
)

var (
	ErrProviderIdentity        = errors.New("model selection: invalid provider identity")
	ErrModelIdentity           = errors.New("model selection: invalid model identity")
	ErrReasoningEffortIdentity = errors.New("model selection: invalid reasoning effort identity")
)

type ProviderIdentity struct{ value string }

func NewProviderIdentity(value string) (ProviderIdentity, error) {
	if err := validateIdentity(value, MaximumProviderIdentityCharacters, ErrProviderIdentity); err != nil {
		return ProviderIdentity{}, err
	}
	return ProviderIdentity{value: value}, nil
}

func (i ProviderIdentity) String() string { return i.value }

type ModelIdentity struct{ value string }

func NewModelIdentity(value string) (ModelIdentity, error) {
	if err := validateIdentity(value, MaximumModelIdentityCharacters, ErrModelIdentity); err != nil {
		return ModelIdentity{}, err
	}
	return ModelIdentity{value: value}, nil
}

func (i ModelIdentity) String() string { return i.value }

type ReasoningEffortIdentity struct{ value string }

func NewReasoningEffortIdentity(value string) (ReasoningEffortIdentity, error) {
	if err := validateIdentity(value, MaximumReasoningEffortCharacters, ErrReasoningEffortIdentity); err != nil {
		return ReasoningEffortIdentity{}, err
	}
	return ReasoningEffortIdentity{value: value}, nil
}

func (i ReasoningEffortIdentity) String() string { return i.value }

func validateIdentity(value string, maximumCharacters int, identityError error) error {
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
