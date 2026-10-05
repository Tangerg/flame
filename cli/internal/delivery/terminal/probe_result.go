package terminal

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/protocol"
)

func providerProbeMessage(providerID string, outcome protocol.ProviderTestOutcome) (string, error) {
	switch outcome {
	case protocol.ProviderTestReachable:
		return "provider " + providerID + " is reachable", nil
	case protocol.ProviderTestNotConfigured:
		return "provider " + providerID + " failed: provider configuration is incomplete; configure its credentials and required endpoint", nil
	case protocol.ProviderTestInvalidCredentials:
		return "provider " + providerID + " failed: credentials were rejected; update the provider API key and check its permissions", nil
	case protocol.ProviderTestTimedOut:
		return "provider " + providerID + " failed: the probe timed out; check the endpoint and connectivity before testing again", nil
	case protocol.ProviderTestFailed:
		return "provider " + providerID + " failed: the probe failed; check the configuration and runtime diagnostics", nil
	default:
		return "", fmt.Errorf("runtime contract violation: provider test outcome %q", outcome)
	}
}

func mcpProbeMessage(candidate string, outcome protocol.MCPTestOutcome) (string, error) {
	switch outcome {
	case protocol.MCPTestReachable:
		return "MCP candidate is reachable · " + candidate, nil
	case protocol.MCPTestAuthorizationRequired:
		return "MCP candidate failed · authorization is required; sign in to the MCP server or update its credentials", nil
	case protocol.MCPTestTimedOut:
		return "MCP candidate failed · the probe timed out; check the endpoint and connectivity before testing again", nil
	case protocol.MCPTestFailed:
		return "MCP candidate failed · the probe failed; check the configuration and runtime diagnostics", nil
	default:
		return "", fmt.Errorf("runtime contract violation: MCP test outcome %q", outcome)
	}
}
