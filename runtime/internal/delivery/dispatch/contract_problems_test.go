package dispatch

import (
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestProblemCatalogPublishesExactChannelSemantics(t *testing.T) {
	t.Parallel()

	var execution []string
	for _, contract := range ProblemContracts() {
		if slices.Contains(contract.Channels, ProblemChannelExecution) {
			execution = append(execution, contract.Type)
		}
	}
	if !slices.Contains(execution, protocol.ProblemChildRunCanceled) {
		t.Fatalf("execution problems omit %q: %v", protocol.ProblemChildRunCanceled, execution)
	}

	// MCP connection status has its own closed vocabulary; none of it is a
	// ProblemData type, so a status category can never be raised as an error.
	for _, contract := range ProblemContracts() {
		for _, status := range []protocol.MCPStatusProblemType{protocol.MCPStatusAuthorizationRequired, protocol.MCPStatusAuthorizationFailed, protocol.MCPStatusDialFailed, protocol.MCPStatusToolDiscoveryFailed, protocol.MCPStatusConfigurationFailed} {
			if contract.Type == string(status) {
				t.Fatalf("MCP status category %q is also a ProblemData type", status)
			}
		}
	}
}
