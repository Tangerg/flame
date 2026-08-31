package runtimeadapter

import (
	"testing"

	cliidentity "github.com/Tangerg/flame/cli/internal/domain/identity"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestResourceIdentityDomainsMatchTheRuntimeContract(t *testing.T) {
	if cliidentity.MaximumResourceCharacters != protocol.MaximumResourceIdentityCharacters ||
		cliidentity.MaximumEventCharacters != protocol.MaximumRunEventIDCharacters {
		t.Fatalf(
			"CLI identity limits = resource:%d event:%d; Runtime = resource:%d event:%d",
			cliidentity.MaximumResourceCharacters,
			cliidentity.MaximumEventCharacters,
			protocol.MaximumResourceIdentityCharacters,
			protocol.MaximumRunEventIDCharacters,
		)
	}
}
