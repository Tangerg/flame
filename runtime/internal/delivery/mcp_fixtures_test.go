package delivery

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

type fakeMCPPorts struct {
	mu            sync.Mutex
	statuses      []mcpserver.ConnectionStatus
	tools         []mcpserver.AdvertisedTool
	reconnectName string
	authorizeName string
	probeErr      error
	conflicts     map[tool.Ref][]tool.Ref
}

func (f *fakeMCPPorts) Statuses() []mcpserver.ConnectionStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.statuses)
}

func (f *fakeMCPPorts) MCPTools(server *mcpserver.ID) ([]mcpserver.AdvertisedTool, map[tool.Ref][]tool.Ref, error) {
	if server == nil {
		return slices.Clone(f.tools), f.conflicts, nil
	}
	var out []mcpserver.AdvertisedTool
	for _, t := range f.tools {
		if t.Server == *server {
			out = append(out, t)
		}
	}
	return out, f.conflicts, nil
}

type fakeMCPAttempt struct{ run func() error }

func (a fakeMCPAttempt) Run() error { return a.run() }
func (fakeMCPAttempt) Close()       {}
func (f *fakeMCPPorts) begin(name mcpserver.ID) mcpapp.Attempt {
	f.mu.Lock()
	f.statuses = slices.DeleteFunc(f.statuses, func(s mcpserver.ConnectionStatus) bool { return s.Server == name })
	f.statuses = append(f.statuses, mcpserver.ConnectionStatus{Server: name, State: mcpserver.ConnectionConnecting})
	f.mu.Unlock()
	return fakeMCPAttempt{run: func() error {
		f.mu.Lock()
		defer f.mu.Unlock()
		for i := range f.statuses {
			if f.statuses[i].Server == name {
				f.statuses[i].State = mcpserver.ConnectionConnected
			}
		}
		return nil
	}}
}
func (f *fakeMCPPorts) BeginConnect(_ context.Context, name mcpserver.ID) (mcpapp.Attempt, error) {
	f.reconnectName = name.Name().String()
	return f.begin(name), nil
}
func (f *fakeMCPPorts) BeginAuthorize(_ context.Context, name mcpserver.ID) (mcpapp.Attempt, error) {
	f.authorizeName = name.Name().String()
	return f.begin(name), nil
}
func (f *fakeMCPPorts) Probe(context.Context, mcpserver.Server) error { return f.probeErr }
func (f *fakeMCPPorts) Detach(name mcpserver.ID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses = slices.DeleteFunc(f.statuses, func(s mcpserver.ConnectionStatus) bool { return s.Server == name })
	return nil
}
func (f *fakeMCPPorts) Refuse(context.Context, mcpserver.ID, mcpserver.ConnectionFailure) error {
	return nil
}

func fakeMCPPortsConfig(ports *fakeMCPPorts) mcpapp.Config {
	servers := make(map[mcpserver.ID]mcpserver.Server, len(ports.statuses))
	for _, status := range ports.statuses {
		servers[status.Server] = mcpserver.Server{
			Source: mcpserver.UserSource(),
			Name:   status.Server.Name(), Enabled: true,
			Transport: mcpserver.TransportStdio, Command: "mcp-" + status.Server.Name().String(),
		}
	}
	registry := &mcpRegistryFake{servers: servers}
	return mcpapp.Config{
		Registry:    registry,
		Store:       registry,
		Connections: ports,
		ToolCatalog: ports,
	}
}

// mcpRegistryFake is the integration registry the MCP config handlers drive.
type mcpRegistryFake struct {
	mu       sync.Mutex
	servers  map[mcpserver.ID]mcpserver.Server
	getErr   error
	saved    []mcpserver.Server
	exposure map[tool.Ref]bool
}

func (m *mcpRegistryFake) Definitions(context.Context) ([]mcpserver.Server, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mcpserver.Server, 0, len(m.servers))
	for _, srv := range m.servers {
		out = append(out, srv.Clone())
	}
	slices.SortFunc(out, func(a, b mcpserver.Server) int {
		return a.ID().Compare(b.ID())
	})
	return out, nil
}

func (m *mcpRegistryFake) Definition(_ context.Context, name mcpserver.ID) (mcpserver.Server, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return mcpserver.Server{}, false, m.getErr
	}
	srv, ok := m.servers[name]
	return srv.Clone(), ok, nil
}

func (m *mcpRegistryFake) Save(_ context.Context, srv mcpserver.Server) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.servers == nil {
		m.servers = make(map[mcpserver.ID]mcpserver.Server)
	}
	m.servers[srv.ID()] = srv.Clone()
	m.saved = append(m.saved, srv.Clone())
	return nil
}

func (m *mcpRegistryFake) Remove(_ context.Context, name mcpserver.ServerName) error {
	id, err := mcpserver.NewID(mcpserver.UserOrigin(), name)
	if err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.servers, id)
	m.mu.Unlock()
	return nil
}

// handlerWithMCP builds a Handler whose capabilities coordinator is wired for the
// MCP handlers (live pool + registry + policy), plus the workspace event hub the
// reconnect/configure paths publish through — bridged like the composition root
// via a neutral signal so the coordinator's connecting → settled frames
// reach the hub.
func handlerWithMCP(t testing.TB, cfg mcpapp.Config) *Handler {
	t.Helper()
	ports := &fakeMCPPorts{}
	if cfg.Registry == nil {
		cfg.Registry = &mcpRegistryFake{}
	}
	if cfg.Store == nil {
		cfg.Store = cfg.Registry.(*mcpRegistryFake)
	}
	if cfg.Connections == nil {
		cfg.Connections = ports
	}
	if cfg.ToolCatalog == nil {
		cfg.ToolCatalog = ports
	}
	if cfg.Exposure == nil {
		cfg.Exposure = mcpapp.NewExposureState(nil, nil)
	}
	mcpInvalidations := &testNotification[invalidation.Notice]{}
	cfg.Invalidations = mcpInvalidations.Publish
	coordinator, err := mcpapp.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		coordinator.BeginShutdown()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := coordinator.AwaitShutdown(ctx); err != nil {
			t.Fatal(err)
		}
	})
	s := &Handler{mcp: coordinator, workspaceHub: newWorkspaceHub()}
	s.observeInvalidations(mcpInvalidations.Observe)
	return s
}

func (m *mcpRegistryFake) ListExposure(context.Context) ([]tool.Ref, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var refs []tool.Ref
	for ref := range m.exposure {
		refs = append(refs, ref)
	}
	return refs, nil
}
func (m *mcpRegistryFake) SetToolExposure(_ context.Context, ref tool.Ref, disabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.exposure == nil {
		m.exposure = make(map[tool.Ref]bool)
	}
	if disabled {
		m.exposure[ref] = true
	} else {
		delete(m.exposure, ref)
	}
	return nil
}
