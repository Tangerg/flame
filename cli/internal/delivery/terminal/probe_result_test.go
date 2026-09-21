package terminal

import (
	"github.com/Tangerg/flame/runtime/protocol"
	"strings"
	"testing"
)

func TestProbeMessagesExplainRecoveryWithoutEchoingDiagnostics(t *testing.T) {
	for _, tt := range []struct{ kind, action string }{
		{protocol.ProblemInvalidAPIKey, "API key"},
		{protocol.ProblemMCPAuthorizationRequired, "sign in"},
		{protocol.ProblemTimeout, "connectivity"},
		{protocol.ProblemProviderNotConfigured, "configure"},
		{protocol.ProblemProviderTestFailed, "diagnostics"},
		{protocol.ProblemMCPDialFailed, "diagnostics"},
	} {
		t.Run(tt.kind, func(t *testing.T) {
			got := probeFailureMessage(&protocol.ProblemData{Type: tt.kind, Detail: "secret-token"})
			if !strings.Contains(got, tt.action) || strings.Contains(got, "secret-token") {
				t.Fatalf("message = %q", got)
			}
		})
	}
}
