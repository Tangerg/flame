package delivery

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/sessions"
	"github.com/Tangerg/flame/runtime/internal/productidentity"
)

func TestNewReportsMissingIntegrations(t *testing.T) {
	_, err := NewHandler(HandlerConfig{Sessions: &sessions.Coordinator{}})
	if err == nil || err.Error() != "delivery: MCP is required" {
		t.Fatalf("New without MCP = %v, want named dependency error", err)
	}
}

func TestServerInfoDefaultUsesCanonicalProductIdentity(t *testing.T) {
	got := (HandlerConfig{}).withServerInfoDefaults().ServerInfo.Name
	if got != productidentity.Name {
		t.Fatalf("default server brand = %q, want %q", got, productidentity.Name)
	}
}
