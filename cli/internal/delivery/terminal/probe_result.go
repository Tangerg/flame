package terminal

import (
	"github.com/Tangerg/flame/cli/internal/domain/failure"
	"github.com/Tangerg/flame/runtime/protocol"
)

func probeFailureMessage(problem *protocol.ProblemData) string {
	if problem == nil {
		return "probe failed"
	}
	switch problem.Type {
	case protocol.ProblemInvalidAPIKey:
		return "credentials were rejected; update the provider API key and check its permissions"
	case protocol.ProblemMCPAuthorizationRequired:
		return "authorization is required; sign in to the MCP server or update its credentials"
	case protocol.ProblemTimeout:
		return "the probe timed out; check the endpoint and connectivity before testing again"
	case protocol.ProblemProviderNotConfigured:
		return "provider configuration is incomplete; configure its credentials and required endpoint"
	case protocol.ProblemProviderTestFailed, protocol.ProblemMCPDialFailed:
		return "the probe failed; check the configuration and runtime diagnostics"
	default:
		return failure.String(problem)
	}
}
