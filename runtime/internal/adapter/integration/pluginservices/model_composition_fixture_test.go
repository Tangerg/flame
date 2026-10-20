package pluginservices

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	hostsqlite "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func installedCallCompositionFixture(t *testing.T, contract *capability.Contract, members []string, register func(*pluginsdk.Backend), handlers ...http.Handler) (*CallComposition, *toolInstallationStore) {
	t.Helper()
	name := testsupport.ServerName("observer")
	contributions := make([]plugin.Contribution, len(members))
	for i, id := range members {
		contributions[i] = plugin.Contribution{ID: plugin.ContributionID(id), Server: name, Export: "observer", Contract: contract.Identity()}
	}
	release, err := plugin.NewRelease(testsupport.Digest("1"), plugin.Declaration{Name: "example.model-composition", Servers: []plugin.Server{{Name: name, Transport: mcpserver.TransportStdio, Command: "observer"}}, Contributions: contributions})
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), "test source", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	source, err := installation.ServerSource(release, name)
	if err != nil {
		t.Fatal(err)
	}
	store := &toolInstallationStore{snapshot: plugin.Snapshot{Installation: installation, Selected: release}}
	remote := sdk.NewServer(&sdk.Implementation{Name: "installed-model-composition"}, nil)
	backend, err := pluginsdk.NewBackend(remote)
	if err != nil {
		t.Fatal(err)
	}
	register(backend)
	if err := backend.BindCapability("observer", contract); err != nil {
		t.Fatal(err)
	}
	if err := backend.Publish(); err != nil {
		t.Fatal(err)
	}
	endpoint := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
	t.Cleanup(endpoint.Close)
	server := mcpserver.Server{Source: source, Name: name, Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: endpoint.URL}
	callbacks := pluginsdk.NewCallbacks()
	t.Cleanup(func() { _ = callbacks.Close() })
	if len(handlers) > 0 {
		if err := callbacks.PublishRPC(handlers[0]); err != nil {
			t.Fatal(err)
		}
	}
	pool, err := mcpconnection.Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, toolSourceRegistry{server}, callbacks, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Shutdown(context.WithoutCancel(t.Context())) })
	attempt, err := pool.BeginConnect(t.Context(), server.ID())
	if err != nil {
		t.Fatal(err)
	}
	defer attempt.Close()
	if err := attempt.Run(); err != nil {
		t.Fatal(err)
	}
	db, err := hostsqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	compositions := hostsqlite.NewCompositionStore(db)
	selections := hostsqlite.NewProviderSelectionStore(db)
	if contract.Descriptor().Composition == capability.Selection {
		choice, err := plugin.NewProviderSelection(plugin.SelectionRecord{Role: contract.Descriptor().Reference, Installation: installation.ID(), Contribution: plugin.ContributionID(members[0])})
		if err != nil {
			t.Fatal(err)
		}
		if err := selections.Save(t.Context(), choice); err != nil {
			t.Fatal(err)
		}
	} else {
		order := make([]plugin.CompositionMember, len(members))
		for i, id := range members {
			order[i] = plugin.CompositionMember{Installation: installation.ID(), Contribution: plugin.ContributionID(id)}
		}
		choice, err := plugin.NewOrderedComposition(plugin.CompositionRecord{Contract: contract.Identity(), Members: order})
		if err != nil {
			t.Fatal(err)
		}
		if err := compositions.Save(t.Context(), choice); err != nil {
			t.Fatal(err)
		}
	}
	routing, err := plugins.NewCommands(hostsqlite.NewPluginCommandStore(db), plugins.NewAdmission())
	if err != nil {
		t.Fatal(err)
	}
	calls, err := NewCommandCalls(store, selections, compositions, pool, routing, nil)
	if err != nil {
		t.Fatal(err)
	}
	composition, err := NewCallComposition(calls, callbacks, plugins.NewAdmission())
	if err != nil {
		t.Fatal(err)
	}
	return composition, store
}
