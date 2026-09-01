package protocol

import (
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// Public model-identity envelopes used by generated clients and local
// consumers before a value crosses the Runtime boundary.
const (
	MaximumProviderIdentityCharacters        = runtimeidentity.MaximumProviderCharacters
	MaximumModelIdentityCharacters           = runtimeidentity.MaximumModelCharacters
	MaximumReasoningEffortIdentityCharacters = runtimeidentity.MaximumReasoningEffortCharacters
)

// ErrIncompleteModelSelection reports a provider/model pair where only one
// identity was supplied.
var ErrIncompleteModelSelection = modelref.ErrIncomplete

// ValidateProviderIdentity reports whether value is an exact provider identity.
func ValidateProviderIdentity(value string) error {
	_, err := modelref.NewProviderIdentity(value)
	return err
}

// ValidateModelIdentity reports whether value is an exact model identity.
func ValidateModelIdentity(value string) error {
	_, err := modelref.NewModelIdentity(value)
	return err
}

// ValidateReasoningEffortIdentity reports whether value is an exact model-owned
// reasoning-effort identity.
func ValidateReasoningEffortIdentity(value string) error {
	_, err := modelref.NewReasoningEffortIdentity(value)
	return err
}

// ValidateModelSelection reports whether provider, model, and reasoning effort
// form an unset or exact complete Runtime model selection.
func ValidateModelSelection(provider, model, reasoningEffort string) error {
	_, err := modelref.NewWithReasoningEffort(provider, model, reasoningEffort)
	return err
}
