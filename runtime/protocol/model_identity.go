package protocol

import (
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
var ErrIncompleteModelSelection = runtimeidentity.ErrIncompleteModelSelection

// ValidateProviderIdentity reports whether value is an exact provider identity.
func ValidateProviderIdentity(value string) error {
	return runtimeidentity.ValidateProviderIdentity(value)
}

// ValidateModelIdentity reports whether value is an exact model identity.
func ValidateModelIdentity(value string) error {
	return runtimeidentity.ValidateModelIdentity(value)
}

// ValidateReasoningEffortIdentity reports whether value is an exact model-owned
// reasoning-effort identity.
func ValidateReasoningEffortIdentity(value string) error {
	return runtimeidentity.ValidateReasoningEffortIdentity(value)
}

// ValidateModelSelection reports whether provider, model, and reasoning effort
// form an unset or exact complete Runtime model selection.
func ValidateModelSelection(provider, model, reasoningEffort string) error {
	return runtimeidentity.ValidateModelSelection(provider, model, reasoningEffort)
}
