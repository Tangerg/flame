package server

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/sessions"
	"github.com/Tangerg/flame/runtime/internal/productidentity"
)

func TestNewReportsMissingIntegrations(t *testing.T) {
	_, err := New(Config{Sessions: &sessions.Coordinator{}})
	if err == nil || err.Error() != "server: MCP is required" {
		t.Fatalf("New without MCP = %v, want named dependency error", err)
	}
}

func TestServerInfoDefaultUsesCanonicalProductIdentity(t *testing.T) {
	got := (Config{}).withServerInfoDefaults().ServerInfo.Name
	if got != productidentity.Name {
		t.Fatalf("default server brand = %q, want %q", got, productidentity.Name)
	}
}
