package delivery

import (
	"cmp"
	"context"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/mcpserver"
)

// fakeMCPPorts implements the four narrow MCP projections consumed by the
// integration use cases. Registry-mutation methods are inert because the
// configuration tests drive a separate durable registry fake.
type fakeMCPPorts struct {
	statuses      []mcpserver.ConnectionStatus
	tools         []mcpserver.AdvertisedTool
	reconnectName string
	authorizeName string
}

func (f *fakeMCPPorts) Statuses() []mcpserver.ConnectionStatus { return f.statuses }

func (f *fakeMCPPorts) Tools(_ context.Context, server *mcpserver.ServerName) ([]mcpserver.AdvertisedTool, error) {
	if server == nil {
		return f.tools, nil
	}
	var out []mcpserver.AdvertisedTool
	for _, t := range f.tools {
		if t.Server == *server {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeMCPPorts) Reconnect(_ context.Context, name mcpserver.ServerName) error {
	f.reconnectName = name.String()
	return nil
}

func (f *fakeMCPPorts) Authorize(_ context.Context, name mcpserver.ServerName) error {
	f.authorizeName = name.String()
	return nil
}

func (*fakeMCPPorts) Probe(context.Context, mcpserver.Server) error     { return nil }
func (*fakeMCPPorts) Configure(context.Context, mcpserver.Server) error { return nil }
func (*fakeMCPPorts) Detach(mcpserver.ServerName) error                 { return nil }

func fakeMCPPortsConfig(ports *fakeMCPPorts) mcpapp.Config {
	servers := make(map[mcpserver.ServerName]mcpserver.Server, len(ports.statuses))
	for _, status := range ports.statuses {
		servers[status.Name] = mcpserver.Server{
			Name: status.Name, Enabled: true,
			Transport: mcpserver.TransportStdio, Command: "mcp-" + status.Name.String(),
		}
	}
	return mcpapp.Config{
		Registry:            &mcpRegistryFake{servers: servers},
		StatusReader:        ports,
		ToolCatalog:         ports,
		ConnectionControl:   ports,
		ConnectionLifecycle: ports,
	}
}

// mcpRegistryFake is the integration registry the MCP config handlers drive.
type mcpRegistryFake struct {
	mu      sync.Mutex
	servers map[mcpserver.ServerName]mcpserver.Server
	getErr  error
	saved   []mcpserver.Server
}

func (m *mcpRegistryFake) List(context.Context) ([]mcpserver.Server, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mcpserver.Server, 0, len(m.servers))
	for _, srv := range m.servers {
		out = append(out, srv)
	}
	slices.SortFunc(out, func(a, b mcpserver.Server) int {
		return cmp.Compare(a.Name.String(), b.Name.String())
	})
	return out, nil
}

func (m *mcpRegistryFake) Get(_ context.Context, name mcpserver.ServerName) (mcpserver.Server, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return mcpserver.Server{}, false, m.getErr
	}
	srv, ok := m.servers[name]
	return srv, ok, nil
}

func (m *mcpRegistryFake) Save(_ context.Context, srv mcpserver.Server) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.servers == nil {
		m.servers = make(map[mcpserver.ServerName]mcpserver.Server)
	}
	m.servers[srv.Name] = srv
	m.saved = append(m.saved, srv)
	return nil
}

func (m *mcpRegistryFake) Remove(_ context.Context, name mcpserver.ServerName) error {
	m.mu.Lock()
	delete(m.servers, name)
	m.mu.Unlock()
	return nil
}

// handlerWithMCP builds a Handler whose capabilities coordinator is wired for the
// MCP handlers (live pool + registry + policy), plus the workspace event hub the
// reconnect/configure paths publish through — bridged like the composition root
// via a neutral signal so the coordinator's connecting → settled frames
// reach the hub.
func handlerWithMCP(cfg mcpapp.Config) *Handler {
	if cfg.Policy == nil {
		policy := mcpserver.NewToolPolicy(nil)
		cfg.Policy = mcpapp.NewToolPolicyState(policy)
	}
	mcpInvalidations := &testNotification[invalidation.Notice]{}
	cfg.Invalidations = mcpInvalidations.Publish
	s := &Handler{mcp: mcpapp.New(cfg), workspaceHub: newWorkspaceHub()}
	s.observeInvalidations(mcpInvalidations.Observe)
	return s
}
