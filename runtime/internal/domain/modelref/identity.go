package modelref

import runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"

const (
	MaximumProviderIdentityCharacters = runtimeidentity.MaximumProviderCharacters
	MaximumModelIdentityCharacters    = runtimeidentity.MaximumModelCharacters
	MaximumReasoningEffortCharacters  = runtimeidentity.MaximumReasoningEffortCharacters
)

var (
	ErrProviderIdentity        = runtimeidentity.ErrProviderIdentity
	ErrModelIdentity           = runtimeidentity.ErrModelIdentity
	ErrReasoningEffortIdentity = runtimeidentity.ErrReasoningEffortIdentity
)

type ProviderIdentity struct{ value string }

func NewProviderIdentity(value string) (ProviderIdentity, error) {
	if err := runtimeidentity.ValidateProviderIdentity(value); err != nil {
		return ProviderIdentity{}, err
	}
	return ProviderIdentity{value: value}, nil
}

func (i ProviderIdentity) String() string { return i.value }

type ModelIdentity struct{ value string }

func NewModelIdentity(value string) (ModelIdentity, error) {
	if err := runtimeidentity.ValidateModelIdentity(value); err != nil {
		return ModelIdentity{}, err
	}
	return ModelIdentity{value: value}, nil
}

func (i ModelIdentity) String() string { return i.value }

type ReasoningEffortIdentity struct{ value string }

func NewReasoningEffortIdentity(value string) (ReasoningEffortIdentity, error) {
	if err := runtimeidentity.ValidateReasoningEffortIdentity(value); err != nil {
		return ReasoningEffortIdentity{}, err
	}
	return ReasoningEffortIdentity{value: value}, nil
}

func (i ReasoningEffortIdentity) String() string { return i.value }
