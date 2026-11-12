package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	connectormcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcpconnector/mcp"
	app "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestExternalConnectorRequiresItsOwners(t *testing.T) {
	if connector, err := NewExternalConnector(nil, nil, nil, nil); err == nil || connector != nil {
		t.Fatal("connector admitted missing lifecycle, directory and grant owners")
	}
}

func TestExternalDirectoryAdmissionRequiresUniqueVersionedIdentities(t *testing.T) {
	first := wire.Entry{Name: "first", Revision: "revision-first", LaunchGrant: "grant-first"}
	second := wire.Entry{Name: "second", Revision: "revision-second", LaunchGrant: "grant-second"}
	missingRevision, missingGrant, invalidName := first, first, first
	missingRevision.Revision = ""
	missingGrant.LaunchGrant = ""
	invalidName.Name = ""
	changed := first
	changed.Revision = "different-revision"
	for _, test := range []struct {
		name     string
		selector string
		entries  []wire.Entry
		valid    bool
	}{
		{"empty", "", nil, true},
		{"unique", "", []wire.Entry{first, second}, true},
		{"selected", first.Name, []wire.Entry{first}, true},
		{"absent", first.Name, nil, true},
		{"duplicate", "", []wire.Entry{first, first}, false},
		{"competing_revisions", "", []wire.Entry{first, changed}, false},
		{"missing_revision", "", []wire.Entry{missingRevision}, false},
		{"missing_grant", "", []wire.Entry{missingGrant}, false},
		{"invalid_name", "", []wire.Entry{invalidName}, false},
		{"different_selection", first.Name, []wire.Entry{second}, false},
		{"extra_selection", first.Name, []wire.Entry{first, second}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			connector, use := externalDirectoryFixture(t, wire.Directory{Entries: test.entries})
			directory, err := connector.entries(t.Context(), use, test.selector)
			if test.valid {
				if err != nil || len(directory.Entries) != len(test.entries) {
					t.Fatalf("valid directory: %v", err)
				}
			} else if !errors.Is(err, app.ErrInvalidServerConfiguration) {
				t.Fatalf("invalid directory was not refused: %v", err)
			}
		})
	}
}

func externalDirectoryFixture(t *testing.T, directory wire.Directory) (*ExternalConnector, *connectorUse) {
	t.Helper()
	methods, err := connectormcp.NewMethods()
	if err != nil {
		t.Fatal(err)
	}
	contract, err := plugincontract.NewCapability(plugincontract.CapabilityConfig{
		Reference: wire.Reference, Composition: plugincontract.Selection, Scope: extension.ScopeInstallation,
	}, methods.List.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "external-directory"}, nil)
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	if err := methods.List.RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, pluginmcp.Command[wire.Selector]) (wire.Directory, error) {
		return directory, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := backend.BindCapability(wire.Export, contract); err != nil {
		t.Fatal(err)
	}
	if err := backend.Publish(); err != nil {
		t.Fatal(err)
	}
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := sdk.NewClient(&sdk.Implementation{Name: "runtime"}, nil)
	if err := pluginmcp.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	capability, err := pluginmcp.NewCapabilityBinding(session, wire.Export, contract)
	if err != nil {
		t.Fatal(err)
	}
	return &ExternalConnector{methods: methods}, &connectorUse{
		capability: capability,
		source:     plugins.ContributionBinding{InstallationID: testsupport.InstallationID(t)},
	}
}
