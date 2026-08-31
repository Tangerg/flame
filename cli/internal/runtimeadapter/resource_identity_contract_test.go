package runtimeadapter

import (
	"testing"

	"github.com/Tangerg/flame/cli/internal/runidentity"
	"github.com/Tangerg/flame/cli/internal/sessionidentity"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestResourceIdentityDomainsMatchTheRuntimeContract(t *testing.T) {
	if sessionidentity.MaximumCharacters != protocol.MaximumResourceIdentityCharacters ||
		runidentity.MaximumCharacters != protocol.MaximumResourceIdentityCharacters ||
		runidentity.MaximumEventCharacters != protocol.MaximumRunEventIDCharacters {
		t.Fatalf(
			"CLI resource identity bounds = session %d, run %d, event %d; Runtime contract = resource %d, event %d",
			sessionidentity.MaximumCharacters,
			runidentity.MaximumCharacters,
			runidentity.MaximumEventCharacters,
			protocol.MaximumResourceIdentityCharacters,
			protocol.MaximumRunEventIDCharacters,
		)
	}
}
