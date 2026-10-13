package plugins

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type userMemory map[mcpserver.ServerName]mcpserver.Server

func (m userMemory) List(context.Context) ([]mcpserver.Server, error) {
	var servers []mcpserver.Server
	for _, server := range m {
		servers = append(servers, server)
	}
	return servers, nil
}

func (m userMemory) Get(_ context.Context, name mcpserver.ServerName) (mcpserver.Server, bool, error) {
	server, found := m[name]
	return server, found, nil
}

type identifiedInstallations struct {
	*installationMemory
	id resourceid.InstallationID
}

func (s *identifiedInstallations) Get(ctx context.Context, id resourceid.InstallationID) (plugin.Snapshot, bool, error) {
	if id != s.id {
		return plugin.Snapshot{}, false, nil
	}
	return s.installationMemory.Get(ctx, id)
}

type serverRead struct {
	name  mcpserver.ServerName
	reach Reach
}

func TestRegistryRefusesMissingRequiredBindingBeforeAcquiringLaunchResources(t *testing.T) {
	contract := requirementContract(t, "consumer", capability.Collection)
	dependency := requirementContract(t, "model", capability.Selection)
	root := requirementSnapshot(t, testsupport.InstallationID(t), "consumer", contract, plugin.Requirement{ID: "model", Contract: dependency.Identity()})
	store := requirementInstallations{snapshots: []plugin.Snapshot{root}}
	packages := &singleServerPackages{}
	registry, err := NewRegistry(userMemory{}, store, packages, requirementSelections{})
	if err != nil {
		t.Fatal(err)
	}
	id, err := root.Installation.ServerID(testsupport.ServerName("backend"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Connection(t.Context(), id); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("missing binding admitted a launch: %v", err)
	}
	if len(packages.reads) != 0 || !root.Installation.ServerEnabled(id.Name()) {
		t.Fatal("dependency failure acquired resources or changed desired enablement")
	}
}

func TestRegistryReadFailuresCannotWithdrawInstallationSources(t *testing.T) {
	release := testsupport.Release(t, "1", plugin.Declaration{Name: "package", Servers: []plugin.Server{{Name: testsupport.ServerName("remote"), Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.test/mcp"}}})
	installation, err := plugin.New(testsupport.InstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	id, err := installation.ServerID(testsupport.ServerName("remote"))
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range []struct {
		name string
		err  error
	}{
		{"missing referenced release", fmt.Errorf("selected release: %w", plugin.ErrNotFound)},
		{"failed transaction retirement", errors.Join(plugin.ErrNotFound, errors.New("rollback failed"))},
	} {
		for _, operation := range []string{"definition", "dispatchable", "connection"} {
			t.Run(failure.name+"/"+operation, func(t *testing.T) {
				registry, err := NewRegistry(userMemory{}, &installationMemory{getErr: failure.err}, &singleServerPackages{}, requirementSelections{})
				if err != nil {
					t.Fatal(err)
				}
				switch operation {
				case "definition":
					_, _, err = registry.Definition(t.Context(), id)
				case "dispatchable":
					_, _, err = registry.Dispatchable(t.Context(), id)
				case "connection":
					_, err = registry.Connection(t.Context(), id)
				}
				if !errors.Is(err, failure.err) || errors.Is(err, mcpapp.ErrUnknownServer) {
					t.Fatalf("registry %s = %v, want retained read failure without source withdrawal", operation, err)
				}
			})
		}
	}
}

// singleServerPackages realizes declared servers from the descriptor alone and
// records every package read, so a test can see which reads a lookup made.
type singleServerPackages struct{ reads []serverRead }

func TestRegistryDefinitionsDoNotRealizePackageResources(t *testing.T) {
	release := testsupport.Release(t, "1", plugin.Declaration{Name: "package", Servers: []plugin.Server{
		{Name: testsupport.ServerName("remote"), Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.test/mcp"},
	}})
	installation, err := plugin.New(testsupport.InstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	store := &installationMemory{catalog: releaseMemory{release.Digest(): release}}
	if err := store.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	registry, err := NewRegistry(userMemory{}, store, &singleServerPackages{}, requirementSelections{})
	if err != nil {
		t.Fatal(err)
	}
	servers, err := registry.Definitions(t.Context())
	if err != nil || len(servers) != 1 {
		t.Fatalf("declared registry depends on package realization: %+v, %v", servers, err)
	}
}

func (p *singleServerPackages) Definitions(ctx context.Context, installation *plugin.Installation, release plugin.Release) ([]mcpserver.Server, error) {
	var result []mcpserver.Server
	for _, declared := range release.Declaration().Servers {
		server, _, err := p.Server(ctx, installation, release, declared.Name, Declared)
		if err != nil {
			return nil, err
		}
		result = append(result, server)
	}
	return result, nil
}

func (p *singleServerPackages) Server(_ context.Context, installation *plugin.Installation, release plugin.Release, name mcpserver.ServerName, reach Reach) (mcpserver.Server, bool, error) {
	p.reads = append(p.reads, serverRead{name, reach})
	for _, declared := range release.Declaration().Servers {
		if declared.Name != name {
			continue
		}
		source, err := installation.ServerSource(release, declared.Name)
		if err != nil {
			return mcpserver.Server{}, false, err
		}
		return mcpserver.Server{Source: source, Name: name, Transport: declared.Transport, URL: declared.URL, Enabled: installation.ServerEnabled(declared.Name)}, true, nil
	}
	return mcpserver.Server{}, false, nil
}

func (p *singleServerPackages) Connection(ctx context.Context, installation *plugin.Installation, release plugin.Release, name mcpserver.ServerName) (mcpapp.Launch, bool, error) {
	server, found, err := p.Server(ctx, installation, release, name, Declared)
	return mcpapp.Launch{Server: server}, found, err
}

func TestRegistryResolvesOneServerPerOrigin(t *testing.T) {
	release := testsupport.Release(t, "1", plugin.Declaration{Name: "package", Servers: []plugin.Server{
		{Name: testsupport.ServerName("target"), Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.test/target"},
		{Name: testsupport.ServerName("sibling"), Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.test/sibling"},
		{Name: testsupport.ServerName("off"), Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.test/off"},
	}})
	installation, err := plugin.New(testsupport.InstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Configure(release, plugin.Configuration{Servers: map[mcpserver.ServerName]plugin.ComponentChange{testsupport.ServerName("off"): plugin.DisableComponent}}); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	installations := &identifiedInstallations{installationMemory: &installationMemory{catalog: releaseMemory{release.Digest(): release}}, id: installation.ID()}
	if err := installations.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	userName, err := mcpserver.ParseServerName("local")
	if err != nil {
		t.Fatal(err)
	}
	userOff, err := mcpserver.ParseServerName("paused")
	if err != nil {
		t.Fatal(err)
	}
	missing, err := mcpserver.ParseServerName("missing")
	if err != nil {
		t.Fatal(err)
	}
	users := userMemory{
		userName: {Source: mcpserver.UserSource(), Name: userName, Transport: mcpserver.TransportStdio, Command: "local", Enabled: true},
		userOff:  {Source: mcpserver.UserSource(), Name: userOff, Transport: mcpserver.TransportStdio, Command: "paused"},
	}
	packages := &singleServerPackages{}
	registry, err := NewRegistry(users, installations, packages, requirementSelections{})
	if err != nil {
		t.Fatal(err)
	}
	userID := func(name mcpserver.ServerName) mcpserver.ID {
		id, err := mcpserver.NewID(mcpserver.UserOrigin(), name)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	installed := func(name string) mcpserver.ID {
		id, err := installation.ServerID(testsupport.ServerName(name))
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	removed, err := plugin.New(testsupport.InstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := removed.ServerID(testsupport.ServerName("target"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name      string
		id        mcpserver.ID
		found     bool
		enabled   bool
		refusal   error
		packageAt bool
	}{
		{name: "user", id: userID(userName), found: true, enabled: true},
		{name: "disabled user", id: userID(userOff), found: true, refusal: mcpapp.ErrServerDisabled},
		{name: "unknown user", id: userID(missing), refusal: mcpapp.ErrUnknownServer},
		{name: "installation", id: installed("target"), found: true, enabled: true, packageAt: true},
		{name: "disabled installation server", id: installed("off"), found: true, refusal: mcpapp.ErrServerDisabled, packageAt: true},
		{name: "undeclared installation server", id: installed("missing"), refusal: mcpapp.ErrUnknownServer, packageAt: true},
		{name: "unknown installation", id: orphan, refusal: mcpapp.ErrUnknownServer},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for reach, read := range map[Reach]func() (mcpserver.Server, bool, error){
				Declared:     func() (mcpserver.Server, bool, error) { return registry.Definition(t.Context(), tt.id) },
				Dispatchable: func() (mcpserver.Server, bool, error) { return registry.Dispatchable(t.Context(), tt.id) },
			} {
				packages.reads = nil
				server, found, err := read()
				if err != nil || found != tt.found || (found && (server.ID() != tt.id || server.Enabled != tt.enabled)) {
					t.Fatalf("reach %d = %+v, %v, %v", reach, server, found, err)
				}
				want := []serverRead(nil)
				if tt.packageAt {
					want = []serverRead{{tt.id.Name(), reach}}
				}
				if len(packages.reads) != len(want) || (len(want) == 1 && packages.reads[0] != want[0]) {
					t.Fatalf("reach %d package reads = %+v, want only %+v", reach, packages.reads, want)
				}
			}
			packages.reads = nil
			server, err := registry.Connection(t.Context(), tt.id)
			if !errors.Is(err, tt.refusal) || (tt.refusal == nil && server.Server.ID() != tt.id) {
				t.Fatalf("connection = %+v, %v; want %v", server, err, tt.refusal)
			}
			if tt.packageAt && (len(packages.reads) != 1 || packages.reads[0] != (serverRead{tt.id.Name(), Declared})) {
				t.Fatalf("connection package reads = %+v, want only the connection target", packages.reads)
			}
		})
	}
}
