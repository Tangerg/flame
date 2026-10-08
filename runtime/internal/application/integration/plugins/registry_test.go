package plugins

import (
	"context"
	"errors"
	"testing"

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

func (s *identifiedInstallations) Get(ctx context.Context, id resourceid.InstallationID) (plugin.Snapshot, error) {
	if id != s.id {
		return plugin.Snapshot{}, plugin.ErrNotFound
	}
	return s.installationMemory.Get(ctx, id)
}

type serverRead struct {
	name  mcpserver.ServerName
	reach Reach
}

// singleServerPackages realizes declared servers from the descriptor alone and
// records every package read, so a test can see which reads a lookup made.
type singleServerPackages struct{ reads []serverRead }

func (p *singleServerPackages) Realize(context.Context, *plugin.Installation, plugin.Release) (Realization, error) {
	return Realization{}, errors.New("unexpected realization of every server")
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
	if err := installation.Approve(release); err != nil {
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
	registry, err := NewRegistry(users, installations, packages)
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
			if !errors.Is(err, tt.refusal) || (tt.refusal == nil && server.ID() != tt.id) {
				t.Fatalf("connection = %+v, %v; want %v", server, err, tt.refusal)
			}
			if tt.packageAt && (len(packages.reads) != 1 || packages.reads[0] != (serverRead{tt.id.Name(), Launchable})) {
				t.Fatalf("connection package reads = %+v, want only the launchable target", packages.reads)
			}
		})
	}
}
