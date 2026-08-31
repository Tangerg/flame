package runtimebinding

import (
	"testing"

	cliidentity "github.com/Tangerg/flame/cli/internal/domain/identity"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestModelIdentityPolicyMatchesRuntimeContract(t *testing.T) {
	t.Parallel()

	if cliidentity.MaximumProviderCharacters != protocol.MaximumProviderIdentityCharacters ||
		cliidentity.MaximumModelCharacters != protocol.MaximumModelIdentityCharacters ||
		cliidentity.MaximumReasoningEffortCharacters != protocol.MaximumReasoningEffortIdentityCharacters {
		t.Fatalf(
			"CLI identity limits = provider:%d model:%d effort:%d; Runtime = provider:%d model:%d effort:%d",
			cliidentity.MaximumProviderCharacters,
			cliidentity.MaximumModelCharacters,
			cliidentity.MaximumReasoningEffortCharacters,
			protocol.MaximumProviderIdentityCharacters,
			protocol.MaximumModelIdentityCharacters,
			protocol.MaximumReasoningEffortIdentityCharacters,
		)
	}
}
