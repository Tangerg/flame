package delivery

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

func TestNewReportsMissingIntegrations(t *testing.T) {
	_, err := NewHandler(HandlerConfig{Sessions: &sessions.Coordinator{}})
	if err == nil || err.Error() != "delivery: MCP is required" {
		t.Fatalf("New without MCP = %v, want named dependency error", err)
	}
}

func TestNewRejectsTypedNilRequiredCapability(t *testing.T) {
	var sessionsCapability *sessions.Coordinator
	_, err := NewHandler(HandlerConfig{Sessions: sessionsCapability})
	if err == nil || err.Error() != "delivery: Sessions is required" {
		t.Fatalf("New with typed-nil Sessions = %v, want named dependency error", err)
	}
}

func TestServerInfoDefaultUsesCanonicalProductIdentity(t *testing.T) {
	got := (HandlerConfig{}).withServerInfoDefaults().ServerInfo.Name
	if got != runtimeidentity.ProductName {
		t.Fatalf("default server brand = %q, want %q", got, runtimeidentity.ProductName)
	}
}
