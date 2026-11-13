package mcp

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	connectormcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcpconnector/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/frame"
)

func TestLateDirectoryReadCannotEraseTheCurrentConnectorSelection(t *testing.T) {
	methods, err := connectormcp.NewMethods()
	if err != nil {
		t.Fatal(err)
	}
	name := testsupport.ServerName("connector")
	release := testsupport.Release(t, "connector", plugin.Declaration{Name: "example.connector", Servers: []plugin.Server{{Name: name, Startup: plugin.DeclaredConnection{Connection: mcpserver.StdioConnection{Command: "connector"}}}}, Contributions: []plugin.Contribution{{ID: plugin.ContributionID(wire.Export), Server: name, Export: wire.Export, Contract: methods.Contract.Identity()}}})
	installation, err := plugin.New(testsupport.InstallationID(t), "test source", release)
	if err != nil {
		t.Fatal(err)
	}
	choice, err := plugin.NewProviderSelection(plugin.SelectionRecord{Role: wire.Reference, Installation: installation.ID(), Contribution: plugin.ContributionID(wire.Export)})
	if err != nil {
		t.Fatal(err)
	}
	selections := &observedConnectorSelections{captured: make(chan struct{}), resume: make(chan struct{})}
	host, err := frame.NewHost(t.Context(), frame.HostConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = host.Close(context.Background()) })
	connector := &ExternalConnector{installed: &Installed{host: host}, installations: inactiveConnectorInstallation{snapshot: plugin.Snapshot{Installation: installation, Selected: release}}, selections: selections, methods: methods}
	older := make(chan error, 1)
	go func() {
		_, err := connector.acquire(t.Context())
		older <- err
	}()
	<-selections.captured
	if err := selections.Save(t.Context(), choice); err != nil {
		t.Fatal(err)
	}
	if _, err := connector.acquire(t.Context()); err != nil {
		t.Fatal(err)
	}
	close(selections.resume)
	if err := <-older; err != nil {
		t.Fatal(err)
	}
	server, err := installation.ServerID(name)
	if err != nil {
		t.Fatal(err)
	}
	affected, err := connector.Affected(t.Context(), []mcpserver.ID{server})
	if err != nil {
		t.Fatal(err)
	}
	if !affected {
		t.Fatal("late directory read erased the current connector selection")
	}
}

type unreadableConnectorSelection struct {
	plugins.SelectionStore
	failure error
}

func (s unreadableConnectorSelection) Get(context.Context, string) (plugin.ProviderSelection, bool, error) {
	return plugin.ProviderSelection{}, false, s.failure
}

func TestConnectorImpactPreservesAnUnreadableSelection(t *testing.T) {
	cause := errors.New("selection owner is unreadable")
	connector := &ExternalConnector{selections: unreadableConnectorSelection{failure: cause}}
	server := testsupport.InstallationMCPServer(testsupport.InstallationID(t), "connector")
	if _, err := connector.Affected(t.Context(), []mcpserver.ID{server}); !errors.Is(err, cause) {
		t.Fatalf("unreadable selection became an unaffected source: %v", err)
	}
	if affected, err := connector.Affected(t.Context(), nil); affected || err != nil {
		t.Fatalf("empty change required a selection read: %t, %v", affected, err)
	}
}

type observedConnectorSelections struct {
	plugins.SelectionStore
	mu       sync.Mutex
	choice   *plugin.ProviderSelection
	first    atomic.Bool
	captured chan struct{}
	resume   chan struct{}
}

func (s *observedConnectorSelections) Get(ctx context.Context, _ string) (plugin.ProviderSelection, bool, error) {
	s.mu.Lock()
	choice := s.choice
	s.mu.Unlock()
	if s.first.CompareAndSwap(false, true) {
		close(s.captured)
		select {
		case <-ctx.Done():
			return plugin.ProviderSelection{}, false, ctx.Err()
		case <-s.resume:
		}
	}
	if choice == nil {
		return plugin.ProviderSelection{}, false, nil
	}
	return *choice, true, nil
}

func (s *observedConnectorSelections) Save(_ context.Context, choice plugin.ProviderSelection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.choice = &choice
	return nil
}

type inactiveConnectorInstallation struct {
	plugins.Store
	snapshot plugin.Snapshot
}

func (s inactiveConnectorInstallation) Get(_ context.Context, id identity.InstallationID) (plugin.Snapshot, bool, error) {
	return s.snapshot, id == s.snapshot.Installation.ID(), nil
}
