package runtimeadapter

import (
	"testing"

	"github.com/Tangerg/flame/cli/internal/modelidentity"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestModelIdentityPolicyMatchesRuntimeContract(t *testing.T) {
	t.Parallel()

	if modelidentity.MaximumProviderCharacters != protocol.MaximumProviderIdentityCharacters ||
		modelidentity.MaximumModelCharacters != protocol.MaximumModelIdentityCharacters ||
		modelidentity.MaximumReasoningEffortCharacters != protocol.MaximumReasoningEffortIdentityCharacters {
		t.Fatalf(
			"CLI identity limits = provider:%d model:%d effort:%d; Runtime = provider:%d model:%d effort:%d",
			modelidentity.MaximumProviderCharacters,
			modelidentity.MaximumModelCharacters,
			modelidentity.MaximumReasoningEffortCharacters,
			protocol.MaximumProviderIdentityCharacters,
			protocol.MaximumModelIdentityCharacters,
			protocol.MaximumReasoningEffortIdentityCharacters,
		)
	}
}
