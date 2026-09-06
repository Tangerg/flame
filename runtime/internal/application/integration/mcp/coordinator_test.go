package mcp

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

func testCoordinator(t testing.TB, cfg Config) *Coordinator {
	t.Helper()
	ports := &fakePorts{}
	if cfg.Registry == nil {
		cfg.Registry = &testRegistry{servers: make(map[mcpserver.ServerName]mcpserver.Server)}
	}
	if cfg.StatusReader == nil {
		cfg.StatusReader = ports
	}
	if cfg.ToolCatalog == nil {
		cfg.ToolCatalog = ports
	}
	if cfg.ConnectionControl == nil {
		cfg.ConnectionControl = ports
	}
	if cfg.ConnectionLifecycle == nil {
		cfg.ConnectionLifecycle = ports
	}
	if cfg.Policy == nil {
		cfg.Policy = NewToolPolicyState(mcpserver.ToolPolicy{})
	}
	coordinator, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { requireCoordinatorShutdown(t, coordinator) })
	return coordinator
}

func TestNewRequiresCompleteDependencies(t *testing.T) {
	t.Parallel()
	for name, omit := range map[string]func(*Config){
		"registry":             func(cfg *Config) { cfg.Registry = nil },
		"status reader":        func(cfg *Config) { cfg.StatusReader = nil },
		"tool catalog":         func(cfg *Config) { cfg.ToolCatalog = nil },
		"connection control":   func(cfg *Config) { cfg.ConnectionControl = nil },
		"connection lifecycle": func(cfg *Config) { cfg.ConnectionLifecycle = nil },
		"policy":               func(cfg *Config) { cfg.Policy = nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := configWithPorts(&fakePorts{})
			cfg.Policy = NewToolPolicyState(mcpserver.ToolPolicy{})
			omit(&cfg)
			if coordinator, err := New(cfg); err == nil || coordinator != nil {
				t.Fatalf("New without %s = (%v, %v), want nil/error", name, coordinator, err)
			}
		})
	}
}
