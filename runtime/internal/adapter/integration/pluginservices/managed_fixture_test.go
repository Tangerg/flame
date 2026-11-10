package pluginservices

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport/mcpfixture"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"github.com/Tangerg/frame"
	sdk "github.com/Tangerg/go-sdk/mcp"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/frame/process"
	"github.com/Tangerg/frame/process/runner"
)

// The child forwards transport messages only. Test callbacks and their business
// receipts stay with the original server while Frame owns a real local process.
func TestManagedTransportHelper(t *testing.T) {
	endpoint := os.Getenv("FLAME_TEST_MCP_PROXY")
	if endpoint == "" {
		return
	}
	err := mcpfixture.Forward(context.Background(), endpoint)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

type managedPackages struct {
	server       mcpserver.Server
	digest       framevalue.Digest
	beforeLaunch func(context.Context) error
}

func (p managedPackages) Server(_ context.Context, installation plugin.Installation, release plugin.Release, name mcpserver.ServerName, _ plugins.Reach) (mcpserver.Server, bool, error) {
	if name != p.server.Name {
		return mcpserver.Server{}, false, nil
	}
	source, err := installation.ServerSource(release, name)
	if err != nil {
		return mcpserver.Server{}, false, err
	}
	server := p.server.Clone()
	server.Source = source
	server.Enabled = installation.Enabled() && installation.ServerEnabled(name)
	return server, true, nil
}

func (p managedPackages) Definitions(ctx context.Context, installation plugin.Installation, release plugin.Release) ([]mcpserver.Server, error) {
	server, _, err := p.Server(ctx, installation, release, p.server.Name, plugins.Declared)
	return []mcpserver.Server{server}, err
}

func (p managedPackages) Connection(ctx context.Context, installation plugin.Installation, release plugin.Release, name mcpserver.ServerName) (mcpapp.Launch, bool, error) {
	if p.beforeLaunch != nil {
		if err := p.beforeLaunch(ctx); err != nil {
			return mcpapp.Launch{}, false, err
		}
	}
	server, found, err := p.Server(ctx, installation, release, name, plugins.Dispatchable)
	if err != nil || !found {
		return mcpapp.Launch{}, found, err
	}
	process, ok := server.Connection.(mcpserver.StdioConnection)
	if !ok {
		return mcpapp.Launch{}, false, errors.New("fixture: managed server realized no process")
	}
	lease, err := os.Open(process.Command)
	if err != nil {
		return mcpapp.Launch{}, false, err
	}
	return mcpapp.Launch{Server: server, Stdio: &mcpapp.Stdio{Command: process.Command, Args: process.Args, Env: process.Env, Dir: process.Dir, ExpectedExecutable: p.digest}, Retire: lease}, true, nil
}

type managedFixture struct {
	Stores    *persistencefixture.Stores
	Host      *frame.Host
	Pool      *mcp.Pool
	Routing   *plugins.Commands
	directory string
	store     *toolInstallationStore
	packages  managedPackages
	callbacks *pluginmcp.Callbacks
	prepare   mcp.ToolPreparation
	runner    *runner.Client
}

func newManagedFixture(t *testing.T, store *toolInstallationStore, server mcpserver.Server, callbacks *pluginmcp.Callbacks, prepare mcp.ToolPreparation) *managedFixture {
	t.Helper()
	directory, err := os.MkdirTemp("", "flame-managed-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	if http, ok := server.Connection.(mcpserver.HTTPConnection); ok {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		server.Connection = mcpserver.StdioConnection{
			Command: executable, Args: []string{"-test.run=^TestManagedTransportHelper$"},
			Env: map[string]string{"FLAME_TEST_MCP_PROXY": http.URL}, Dir: directory,
		}
	}
	data, err := os.ReadFile(server.Connection.(mcpserver.StdioConnection).Command)
	if err != nil {
		t.Fatal(err)
	}
	digest := framevalue.Hash(data)
	runnerOwner, err := runner.Open(runner.Config{Directory: filepath.Join(directory, "runner")})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := runnerOwner.Listen(filepath.Join(directory, "runner", "control.sock"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- runnerOwner.Serve(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
		<-served
		if err := runnerOwner.Close(); err != nil {
			t.Error(err)
		}
	})
	client, err := runner.NewClient(filepath.Join(directory, "runner", "control.sock"))
	if err != nil {
		t.Fatal(err)
	}
	fixture := &managedFixture{directory: directory, store: store, packages: managedPackages{server: server, digest: digest}, callbacks: callbacks, prepare: prepare, runner: client}
	t.Cleanup(func() { fixture.close(t) })
	fixture.open(t, true)
	return fixture
}

func (f *managedFixture) open(t *testing.T, connect bool) {
	t.Helper()
	stores, err := persistencefixture.Open(t, context.Background(), filepath.Join(f.directory, "data"))
	if err != nil {
		t.Fatal(err)
	}
	f.Stores = stores
	f.store.Store = stores.Installations
	commands, err := plugins.NewCommands(stores.PluginCommands, stores.Admission)
	if err != nil {
		t.Fatal(err)
	}
	f.Routing = commands
	callers, err := NewCallerRestorer(commands, stores.ToolInvocations)
	if err != nil {
		t.Fatal(err)
	}
	source, err := mcp.NewProcessSource(f.store, f.packages, f.callbacks, callers)
	if err != nil {
		t.Fatal(err)
	}
	driver, err := process.New(process.Config{Runner: f.runner, Store: stores.Frame, Source: source, CloseTimeout: 5 * time.Second, Protocol: source, MaxFrameBytes: sdk.DefaultMaxLineLength})
	if err != nil {
		t.Fatal(err)
	}
	permission, err := mcp.NewCatalogPermission(stores.Admission, stores.PendingPluginBindings)
	if err != nil {
		t.Fatal(err)
	}
	owner := frame.NewLifetime(context.Background())
	f.Host, err = frame.NewHost(context.Background(), frame.HostConfig{ReadAcceptedBinding: permission.Read, Admission: stores.Admission, Epoch: stores.Frame.Epoch(), Lifetime: owner, Drivers: []frame.Driver{driver}, Installations: stores.Frame, ExecutionProtection: stores.Frame})
	if err != nil {
		t.Fatal(err)
	}
	f.store.host = f.Host
	release := f.store.snapshot.Selected
	if err := stores.PluginReleases.Admit(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	definition, err := plugins.ProjectFrameworkRelease(release, pluginmcp.ProcessProfile)
	if err != nil {
		t.Fatal(err)
	}
	if err := plugins.RegisterFrameworkRelease(f.Host, definition); err != nil {
		t.Fatal(err)
	}
	records, err := stores.Frame.Installations(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 {
		configuration, err := plugins.EncodeFrameworkConfiguration(f.store.snapshot.Installation.Snapshot())
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Host.Install(t.Context(), frame.InstallationConfig{ID: f.store.snapshot.Installation.ID().Host(), Plugin: definition.Plugin, Configuration: configuration, Enabled: f.store.snapshot.Installation.Enabled()})
		if err != nil {
			t.Fatal(err)
		}
	} else if err := f.Host.Restore(t.Context(), records, []*frame.Plugin{definition.Plugin}); err != nil {
		t.Fatal(err)
	}
	if err := stores.RecoverPluginBindings(t.Context()); err != nil {
		t.Fatal(err)
	}
	registry, err := plugins.NewRegistry(f.Host, f.store, f.packages, stores.ProviderSelections, stores.PluginCompositions)
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	close(ready)
	installed, err := mcp.NewInstalled(mcp.InstalledConfig{Host: f.Host, Source: source, Registry: registry, Lifetime: owner, Permission: permission, Ready: ready})
	if err != nil {
		t.Fatal(err)
	}
	servers, err := registry.Definitions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	f.Pool, err = mcp.Open(t.Context(), context.Background(), servers, nil, registry, f.callbacks, nil, callers, installed)
	if err != nil {
		t.Fatal(err)
	}
	if !connect {
		return
	}
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		attempt, err := f.Pool.BeginConnect(t.Context(), server.ID())
		if err != nil {
			t.Fatal(err)
		}
		err = attempt.Run()
		attempt.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func (f *managedFixture) close(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if f.Pool != nil {
		if err := f.Pool.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		f.Pool = nil
	}
	if f.Host != nil {
		if outcome, err := f.Host.Close(ctx); err != nil || !outcome.Released {
			t.Errorf("managed Host close: %+v, %v", outcome, err)
		}
		f.Host = nil
	}
	if f.Stores != nil {
		if err := f.Stores.Close(); err != nil {
			t.Error(err)
		}
		f.Stores = nil
	}
}

func (f *managedFixture) reopen(t *testing.T) {
	t.Helper()
	f.close(t)
	f.open(t, true)
}

func (s *toolInstallationStore) disable(t *testing.T) {
	t.Helper()
	if s.host == nil {
		t.Fatal("installation fixture has no Host authority")
	}
	if err := s.host.SetEnabled(t.Context(), s.snapshot.Installation.ID().Host(), false); err != nil {
		t.Fatal(err)
	}
	snapshot, found, err := s.Store.Get(t.Context(), s.snapshot.Installation.ID())
	if err != nil || !found {
		t.Fatalf("disabled installation projection: %t, %v", found, err)
	}
	s.snapshot = snapshot
}
