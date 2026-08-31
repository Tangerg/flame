package protocol

import runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"

// Public model-identity envelopes used by generated clients and local
// consumers before a value crosses the Runtime boundary.
const (
	MaximumProviderIdentityCharacters        = runtimeidentity.MaximumProviderCharacters
	MaximumModelIdentityCharacters           = runtimeidentity.MaximumModelCharacters
	MaximumReasoningEffortIdentityCharacters = runtimeidentity.MaximumReasoningEffortCharacters
)
