package mcp

import (
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestDependenciesUseTheActualTransitiveActivationReleases(t *testing.T) {
	c := &Connections{lifetime: t.Context(), sessions: map[*sdk.ClientSession]*ownedSession{}}
	add := func(label string) (*server, plugin.Dependency) {
		dependency := plugin.Dependency{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest(label)}
		source, err := mcpserver.InstallationSource(dependency.InstallationID, dependency.Digest, testsupport.Digest("authority"), testsupport.Digest("recipient"))
		if err != nil {
			t.Fatal(err)
		}
		config := ServerConfig{Server: mcpserver.Server{Source: source, Name: testsupport.ServerName(label)}}
		entry := &server{id: config.ID(), config: config, session: &sdk.ClientSession{}}
		c.servers = append(c.servers, entry)
		c.sessions[entry.session] = &ownedSession{}
		return entry, dependency
	}
	root, a := add("root")
	middle, b := add("middle")
	leaf, d := add("leaf")
	bind := func(from, to *server, provider plugin.Dependency, optional bool) {
		bindings, err := plugin.NewRequiredBindings([]plugin.ResolvedRequirement{{Contribution: "owner", ID: "dependency", Contract: testsupport.CommandContract(t, "records.update", "installation"), Optional: optional, Bindings: []plugin.BoundContribution{{Provider: provider, Contribution: "owner", Server: to.id.Name(), Export: "records", Contract: testsupport.CommandContract(t, "records.update", "installation")}}}})
		if err != nil {
			t.Fatal(err)
		}
		c.sessions[from.session].bindings = bindings
	}
	bind(root, middle, b, false)
	bind(middle, leaf, d, true)
	// Optional cycles do not create an unbounded dependency traversal.
	bind(leaf, root, a, true)
	want := plugin.CompactDependencies([]plugin.Dependency{a, b, d})
	held, err := c.Dependencies(root.session)
	if err != nil || !slices.Equal(held, want) {
		t.Fatalf("transitive graph = %v, %v; want %v", held, err, want)
	}
	retained := plugin.RetainedReleases(nil, held)
	if len(retained) != 3 {
		t.Fatalf("transitive releases were reclaimable: %v", retained)
	}
	// Returned projections cannot change connection-owned bindings.
	held[0] = plugin.Dependency{}
	if again, err := c.Dependencies(root.session); err != nil || !slices.Equal(again, want) {
		t.Fatalf("caller mutated activation graph: %v, %v", again, err)
	}
	oldSession := leaf.session
	leaf.session = &sdk.ClientSession{}
	c.sessions[leaf.session] = &ownedSession{}
	if _, err := c.Dependencies(oldSession); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("retired generation remained usable: %v", err)
	}
	originalSource := leaf.config.Source
	leaf.config.Source, err = mcpserver.InstallationSource(d.InstallationID, testsupport.Digest("replacement"), testsupport.Digest("authority"), testsupport.Digest("recipient"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Dependencies(root.session); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("replacement release entered the original graph: %v", err)
	}
	leaf.config.Source = originalSource
	leaf.session = nil
	if _, err := c.Dependencies(root.session); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("bound optional failure became absence: %v", err)
	}
}
