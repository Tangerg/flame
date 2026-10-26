package model

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

const (
	MaximumProviderIdentityCharacters = 64
	MaximumModelIdentityCharacters    = 256
	MaximumReasoningEffortCharacters  = 32
)

var (
	ErrProviderIdentity        = errors.New("model selection: invalid provider identity")
	ErrModelIdentity           = errors.New("model selection: invalid model identity")
	ErrReasoningEffortIdentity = errors.New("model selection: invalid reasoning effort identity")
)

type ProviderIdentity struct{ value string }

func NewProviderIdentity(value string) (ProviderIdentity, error) {
	if err := ValidateProviderIdentity(value); err != nil {
		return ProviderIdentity{}, err
	}
	return ProviderIdentity{value: value}, nil
}

func (i ProviderIdentity) String() string { return i.value }

type ModelIdentity struct{ value string }

func NewModelIdentity(value string) (ModelIdentity, error) {
	if err := ValidateModelIdentity(value); err != nil {
		return ModelIdentity{}, err
	}
	return ModelIdentity{value: value}, nil
}

func (i ModelIdentity) String() string { return i.value }

type ReasoningEffortIdentity struct{ value string }

func NewReasoningEffortIdentity(value string) (ReasoningEffortIdentity, error) {
	if err := ValidateReasoningEffortIdentity(value); err != nil {
		return ReasoningEffortIdentity{}, err
	}
	return ReasoningEffortIdentity{value: value}, nil
}

func (i ReasoningEffortIdentity) String() string { return i.value }

func ValidateProviderIdentity(value string) error {
	return validateIdentity(value, MaximumProviderIdentityCharacters, ErrProviderIdentity)
}

func ValidateModelIdentity(value string) error {
	return validateIdentity(value, MaximumModelIdentityCharacters, ErrModelIdentity)
}

func ValidateReasoningEffortIdentity(value string) error {
	return validateIdentity(value, MaximumReasoningEffortCharacters, ErrReasoningEffortIdentity)
}

func ValidateSelection(provider, model, reasoningEffort string) error {
	if (provider == "") != (model == "") {
		return ErrIncomplete
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

func validateIdentity(value string, maximumCharacters int, identityError error) error {
	if err := identity.ValidateText(value, maximumCharacters); err != nil {
		return fmt.Errorf("%w: %v", identityError, err)
	}
	return nil
}
