//go:build unix

package pluginpackage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/google/uuid"
)

type reconnectObserver struct {
	*mcpconnection.Pool
	finished chan error
}

func (p reconnectObserver) Reconnect(ctx context.Context, name mcpserver.ServerName) error {
	err := p.Pool.Reconnect(ctx, name)
	p.finished <- err
	return err
}

type emptyToolDiagnostics struct{}

func (emptyToolDiagnostics) ToolNameConflicts() (map[tool.Ref][]tool.Ref, error) {
	return nil, nil
}

func TestRepairedReleaseReconnectsThroughItsCurrentDefinition(t *testing.T) {
	releases, installations, users := testReleaseStore(t)
	source := writePackage(t, map[string]string{
		"plugin.json": portableManifest,
		"mcp.json":    `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"sh","args":["${PLUGIN_ROOT}/backend.sh"]}}}`,
		"backend.sh":  `printf original > "$PLUGIN_DATA/started"`,
	})
	release, err := releases.Materialize(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(uuid.NewString(), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Approve(release.Digest, nil); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(true); err != nil {
		t.Fatal(err)
	}
	if err := installations.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	registry, err := plugins.NewRegistry(users, installations, releases)
	if err != nil {
		t.Fatal(err)
	}
	name, err := mcpserver.InstallationServer(installation.Snapshot().ID, "backend")
	if err != nil {
		t.Fatal(err)
	}
	server, found, err := registry.Definition(t.Context(), name)
	if err != nil || !found {
		t.Fatalf("source definition = %v, %v", found, err)
	}
	pool, _, err := mcpconnection.Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	marker := filepath.Join(releases.dataRoot(installation.Snapshot().ID), "started")
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	root, err := releases.Root(release.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, root+"-retired"); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(root, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := registry.Get(t.Context(), name); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("replacement directory bypassed retained integrity: %v", err)
	}
	observer := reconnectObserver{Pool: pool, finished: make(chan error, 1)}
	coordinator, err := mcpapp.New(mcpapp.Config{Registry: registry, StatusReader: pool, ToolCatalog: pool, ToolDiagnostics: emptyToolDiagnostics{}, ConnectionControl: observer, ConnectionLifecycle: pool, Exposure: mcpapp.NewExposureState([]mcpserver.Server{server}, nil)})
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
	release, err := releases.Materialize(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(uuid.NewString(), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Approve(release.Digest, nil); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(true); err != nil {
		t.Fatal(err)
	}
	if err := installations.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	if diagnostics, err := releases.Prepare(t.Context(), installation); err != nil || len(diagnostics) != 0 {
		t.Fatalf("prepare = %v, %v", diagnostics, err)
	}
	registry, err := plugins.NewRegistry(userServers, installations, releases)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := registry.Catalog(t.Context())
	if err != nil || len(sources) != 1 {
		t.Fatalf("sources = %v, %v", sources, err)
	}
	pool, _, err := mcpconnection.Open(t.Context(), t.Context(), []mcpserver.Server{sources[0].Server}, nil, registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	marker := filepath.Join(releases.dataRoot(installation.Snapshot().ID), "started")
	if content, err := os.ReadFile(marker); err != nil || string(content) != "original" {
		t.Fatalf("admitted process did not run: %q, %v", content, err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	root, err := releases.Root(release.Digest)
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
		"configure": func() error { return pool.Configure(t.Context(), sources[0].Server.Name) },
		"reconnect": func() error { return pool.Reconnect(t.Context(), sources[0].Server.Name) },
		"authorize": func() error { return pool.Authorize(t.Context(), sources[0].Server.Name) },
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
		pool, tools, err := mcpconnection.Open(t.Context(), t.Context(), []mcpserver.Server{sources[0].Server}, nil, registry)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
				t.Error(err)
			}
		}()
		if statuses := pool.Statuses(); len(tools) != 0 || len(statuses) != 1 || statuses[0].State != mcpserver.ConnectionFailed {
			t.Fatalf("tampered startup = %v, %v", tools, statuses)
		}
		if content, err := os.ReadFile(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("tampered startup ran: %q, %v", content, err)
		}
	})
}
