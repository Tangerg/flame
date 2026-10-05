package mcp

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

func testCoordinator(t testing.TB, cfg Config) *Coordinator {
	t.Helper()
	ports := &fakePorts{}
	if cfg.Registry == nil {
		cfg.Registry = &testRegistry{servers: make(map[mcpserver.ID]mcpserver.Server)}
	}
	if cfg.Store == nil {
		store, isStore := cfg.Registry.(Store)
		if !isStore {
			store = &testRegistry{servers: make(map[mcpserver.ID]mcpserver.Server)}
		}
		cfg.Store = store
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
	if cfg.Exposure == nil {
		cfg.Exposure = NewExposureState(nil, nil)
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
		"registry":               func(cfg *Config) { cfg.Registry = nil },
		"status reader":          func(cfg *Config) { cfg.StatusReader = nil },
		"tool catalog":           func(cfg *Config) { cfg.ToolCatalog = nil },
		"connection control":     func(cfg *Config) { cfg.ConnectionControl = nil },
		"connection lifecycle":   func(cfg *Config) { cfg.ConnectionLifecycle = nil },
		"exposure":               func(cfg *Config) { cfg.Exposure = nil },
		"uninitialized exposure": func(cfg *Config) { cfg.Exposure = &ExposureState{} },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := configWithPorts(&fakePorts{})
			cfg.Exposure = NewExposureState(nil, nil)
			omit(&cfg)
			if coordinator, err := New(cfg); err == nil || coordinator != nil {
				t.Fatalf("New without %s = (%v, %v), want nil/error", name, coordinator, err)
			}
		})
	}
}
