package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestProviderProbeMessagesCoverTheClosedOutcomeSet(t *testing.T) {
	for _, tt := range []struct {
		outcome protocol.ProviderTestOutcome
		action  string
	}{
		{protocol.ProviderTestReachable, "reachable"},
		{protocol.ProviderTestNotConfigured, "configure"},
		{protocol.ProviderTestInvalidCredentials, "API key"},
		{protocol.ProviderTestTimedOut, "connectivity"},
		{protocol.ProviderTestFailed, "diagnostics"},
	} {
		got, err := providerProbeMessage("deepseek", tt.outcome)
		if err != nil || !strings.Contains(got, tt.action) {
			t.Fatalf("%s message = %q, %v", tt.outcome, got, err)
		}
	}
	if got, err := providerProbeMessage("deepseek", "provider_test_failed"); err == nil || !strings.Contains(err.Error(), "contract violation") {
		t.Fatalf("unknown provider outcome = %q, %v; want a contract violation", got, err)
	}
}

func TestMCPProbeMessagesCoverTheClosedOutcomeSet(t *testing.T) {
	for _, tt := range []struct {
		outcome protocol.MCPTestOutcome
		action  string
	}{
		{protocol.MCPTestReachable, "reachable"},
		{protocol.MCPTestAuthorizationRequired, "sign in"},
		{protocol.MCPTestTimedOut, "connectivity"},
		{protocol.MCPTestFailed, "diagnostics"},
	} {
		got, err := mcpProbeMessage("probe", tt.outcome)
		if err != nil || !strings.Contains(got, tt.action) {
			t.Fatalf("%s message = %q, %v", tt.outcome, got, err)
		}
	}
	if got, err := mcpProbeMessage("probe", "mcp_dial_failed"); err == nil || !strings.Contains(err.Error(), "contract violation") {
		t.Fatalf("unknown MCP outcome = %q, %v; want a contract violation", got, err)
	}
}
