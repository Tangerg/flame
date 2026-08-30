package protocol

import "github.com/Tangerg/flame/runtime/internal/modelidentity"

// Public model-identity envelopes used by generated clients and local
// consumers before a value crosses the Runtime boundary.
const (
	MaximumProviderIdentityCharacters        = modelidentity.MaximumProviderCharacters
	MaximumModelIdentityCharacters           = modelidentity.MaximumModelCharacters
	MaximumReasoningEffortIdentityCharacters = modelidentity.MaximumReasoningEffortCharacters
)
