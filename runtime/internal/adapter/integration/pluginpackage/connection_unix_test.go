//go:build unix

package pluginpackage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/testsupport"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type reconnectObserver struct {
	*mcpconnection.Pool
	finished chan error
}

func (p reconnectObserver) Reconnect(ctx context.Context, name mcpserver.ID) error {
	err := p.Pool.Reconnect(ctx, name)
	p.finished <- err
	return err
}

type emptyToolCatalog struct{}

func (emptyToolCatalog) MCPTools(*mcpserver.ID) ([]mcpserver.AdvertisedTool, map[tool.Ref][]tool.Ref, error) {
	return nil, nil, nil
}

func TestRepairedReleaseReconnectsThroughItsCurrentDefinition(t *testing.T) {
	releases, installations, users := testReleaseStore(t)
	source := writePackage(t, map[string]string{
		"plugin.json": portableManifest,
		"mcp.json":    `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"sh","args":["${PLUGIN_ROOT}/backend.sh"]}}}`,
		"backend.sh":  `printf original > "$PLUGIN_DATA/started"`,
	})
	release, err := publishPackage(t.Context(), releases, source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	if err := installations.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	registry, err := plugins.NewRegistry(users, installations, releases)
	if err != nil {
		t.Fatal(err)
	}
	name, err := installation.ServerID(testsupport.ServerName("backend"))
	if err != nil {
		t.Fatal(err)
	}
	server, found, err := registry.Definition(t.Context(), name)
	if err != nil || !found {
		t.Fatalf("source definition = %v, %v", found, err)
	}
	pool, err := mcpconnection.Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	marker := filepath.Join(releases.dataRoot(installation.ID()), "started")
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	root, err := releases.Root(release.Digest())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, root+"-retired"); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(root, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := registry.Dispatchable(t.Context(), name); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("replacement directory bypassed retained integrity: %v", err)
	}
	observer := reconnectObserver{Pool: pool, finished: make(chan error, 1)}
	coordinator, err := mcpapp.New(mcpapp.Config{Registry: registry, Store: users, StatusReader: pool, ToolCatalog: emptyToolCatalog{}, ConnectionControl: observer, ConnectionLifecycle: pool, Exposure: mcpapp.NewExposureState([]mcpserver.Server{server}, nil)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		coordinator.BeginShutdown()
		if err := coordinator.AwaitShutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	if err := coordinator.ReconnectServer(t.Context(), name); err != nil {
		t.Fatalf("old availability blocked fresh admission: %v", err)
	}
	select {
	case <-observer.finished:
	case <-time.After(5 * time.Second):
		t.Fatal("reconnect did not reach the connection owner")
	}
	if content, err := os.ReadFile(marker); err != nil || string(content) != "original" {
		t.Fatalf("repaired package did not launch after fresh integrity validation: %q, %v", content, err)
	}
}

func TestTamperedPackageCannotLaunchThroughConnectionEntrypoints(t *testing.T) {
	releases, installations, userServers := testReleaseStore(t)
	source := writePackage(t, map[string]string{
		"plugin.json": portableManifest,
		"mcp.json":    `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"sh","args":["${PLUGIN_ROOT}/backend.sh"]}}}`,
		"backend.sh":  `printf original > "$PLUGIN_DATA/started"`,
	})
	release, err := publishPackage(t.Context(), releases, source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	if err := installations.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	if err := releases.Prepare(t.Context(), installation, release); err != nil {
		t.Fatalf("prepare = %v", err)
	}
	registry, err := plugins.NewRegistry(userServers, installations, releases)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := registry.Catalog(t.Context())
	if err != nil || len(sources) != 1 {
		t.Fatalf("sources = %v, %v", sources, err)
	}
	pool, err := mcpconnection.Open(t.Context(), t.Context(), []mcpserver.Server{sources[0].Server}, nil, registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	marker := filepath.Join(releases.dataRoot(installation.ID()), "started")
	if content, err := os.ReadFile(marker); err != nil || string(content) != "original" {
		t.Fatalf("admitted process did not run: %q, %v", content, err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	root, err := releases.Root(release.Digest())
	if err != nil {
		t.Fatal(err)
	}
	backend := filepath.Join(root, "backend.sh")
	if err := os.Chmod(backend, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backend, []byte(`printf tampered > "$PLUGIN_DATA/started"`), 0600); err != nil {
		t.Fatal(err)
	}
	for name, connect := range map[string]func() error{
		"configure": func() error { return pool.Configure(t.Context(), sources[0].Server.ID()) },
		"reconnect": func() error { return pool.Reconnect(t.Context(), sources[0].Server.ID()) },
		"authorize": func() error { return pool.Authorize(t.Context(), sources[0].Server.ID()) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := connect(); !errors.Is(err, plugin.ErrUnavailable) {
				t.Fatalf("connection admission = %v, want unavailable release", err)
			}
			if content, err := os.ReadFile(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("tampered process ran: %q, %v", content, err)
			}
		})
	}
	t.Run("startup", func(t *testing.T) {
		pool, err := mcpconnection.Open(t.Context(), t.Context(), []mcpserver.Server{sources[0].Server}, nil, registry)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
				t.Error(err)
			}
		}()
		var tools []toolcontract.Tool
		pool.SetToolSink(func(catalog []toolcontract.Tool) { tools = catalog })
		statuses := pool.Statuses()
		if len(tools) != 0 || len(statuses) != 1 || statuses[0].State != mcpserver.ConnectionFailed {
			t.Fatalf("tampered startup = %v, %v", tools, statuses)
		}
		if statuses[0].Failure != mcpserver.FailureConfiguration {
			t.Fatalf("refused startup admission reached status as %q, want configuration", statuses[0].Failure)
		}
		if content, err := os.ReadFile(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("tampered startup ran: %q, %v", content, err)
		}
	})
}

func TestBackendPreparationFailureIsObservedByLaterReads(t *testing.T) {
	releases, installations, users := testReleaseStore(t)
	source := writePackage(t, map[string]string{
		"plugin.json": portableManifest,
		"mcp.json":    `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"sh","cwd":"${PLUGIN_DATA}/work"},"remote":{"type":"streamable-http","url":"https://example.test/mcp"}}}`,
	})
	release, err := publishPackage(t.Context(), releases, source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	if err := installations.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	data := releases.dataRoot(installation.ID())
	if err := os.MkdirAll(filepath.Dir(data), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(data, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := releases.Prepare(t.Context(), installation, release); err == nil {
		t.Fatal("backend preparation over a non-directory data root succeeded")
	}
	coordinator, err := plugins.New(t.Context(), installations, releases.catalog, releases, unusedConnections{}, unusedDependencies{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := mcpserver.ParseServerName("backend")
	if err != nil {
		t.Fatal(err)
	}
	want := []mcpserver.ServerName{backend}
	for range 2 {
		listed, err := coordinator.List(t.Context())
		if err != nil || len(listed) != 1 || listed[0].Realization.Release != plugins.ReleaseAvailable || !reflect.DeepEqual(listed[0].Realization.UnavailableBackends(), want) {
			t.Fatalf("later listing = %+v, %v; want the failed backend observed", listed, err)
		}
	}
	registry, err := plugins.NewRegistry(users, installations, releases)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := registry.Catalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		want := mcpapp.SourceAvailable
		if source.Server.Name == backend {
			want = mcpapp.SourceUnavailableBackend
		}
		if source.Availability != want {
			t.Fatalf("server %s availability = %q, want %q", source.Server.ID(), source.Availability, want)
		}
	}
	if err := os.Remove(data); err != nil {
		t.Fatal(err)
	}
	if err := releases.Prepare(t.Context(), installation, release); err != nil {
		t.Fatalf("repaired preparation: %v", err)
	}
	listed, err := coordinator.List(t.Context())
	if err != nil || len(listed) != 1 || len(listed[0].Realization.UnavailableBackends()) != 0 {
		t.Fatalf("repaired backend still reads as unavailable: %+v, %v", listed, err)
	}
}

func TestDisabledInstallationServerRefusesConnectionAsDisabled(t *testing.T) {
	releases, installations, users := testReleaseStore(t)
	source := writePackage(t, map[string]string{
		"plugin.json": portableManifest,
		"mcp.json":    `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"sh"},"remote":{"type":"streamable-http","url":"https://example.test/mcp"}}}`,
	})
	release, err := publishPackage(t.Context(), releases, source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err := installation.Configure(release, plugin.Configuration{Servers: map[mcpserver.ServerName]plugin.ComponentChange{testsupport.ServerName("backend"): plugin.DisableComponent}}); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	if err := installations.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	registry, err := plugins.NewRegistry(users, installations, releases)
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := installation.ServerID(testsupport.ServerName("backend"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Connection(t.Context(), disabled); !errors.Is(err, mcpapp.ErrServerDisabled) || errors.Is(err, plugin.ErrUnapproved) {
		t.Fatalf("disabled server connection = %v, want server disabled", err)
	}
	enabled, err := installation.ServerID(testsupport.ServerName("remote"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Connection(t.Context(), enabled); err != nil {
		t.Fatalf("enabled sibling connection = %v", err)
	}
	installation.Disable()
	if err := installations.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Connection(t.Context(), enabled); !errors.Is(err, mcpapp.ErrServerDisabled) {
		t.Fatalf("server of a disabled installation = %v, want server disabled", err)
	}
	undeclared, err := installation.ServerID(testsupport.ServerName("missing"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Connection(t.Context(), undeclared); !errors.Is(err, mcpapp.ErrUnknownServer) {
		t.Fatalf("undeclared server connection = %v, want unknown server", err)
	}
}

type unusedConnections struct{}

func (unusedConnections) WithdrawInstallation([]mcpserver.ID) error {
	return errors.New("unexpected withdrawal")
}

func (unusedConnections) ReconcileInstallation(context.Context, []mcpserver.ID) error {
	return errors.New("unexpected reconciliation")
}

type unusedDependencies struct{}

func (unusedDependencies) ChangeInstallation(context.Context, resourceid.InstallationID, plugins.ChangeAdmission, func([]plugin.Dependency) error) error {
	return errors.New("unexpected installation change")
}

func (unusedDependencies) UnderAdmission(context.Context, func([]plugin.Dependency) error) error {
	return errors.New("unexpected admission")
}
