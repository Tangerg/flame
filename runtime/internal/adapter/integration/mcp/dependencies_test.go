package mcp

import (
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestDependenciesUseTheActualTransitiveActivationReleases(t *testing.T) {
	c := &connections{lifetime: t.Context(), sessions: map[*sdk.ClientSession]*ownedSession{}}
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
	selected, err := capability.Parse(testsupport.CommandContract(t, "records.update", "installation").JSON())
	if err != nil {
		t.Fatal(err)
	}
	contract, err := capability.New(capability.Config{Reference: "records", Composition: capability.Decoration, Scope: "installation"}, selected.Descriptor().Methods...)
	if err != nil {
		t.Fatal(err)
	}
	bind := func(from, to *server, provider plugin.Dependency, optional bool) {
		bindings, err := plugin.NewRequiredBindings([]plugin.ResolvedRequirement{{Contribution: "owner", ID: "dependency", Contract: contract.Identity(), Optional: optional, Bindings: []plugin.BoundContribution{{Provider: provider, Contribution: "owner", Server: to.id.Name(), Export: "records", Contract: contract.Identity()}}}})
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
	held, err := c.activation(root.session)
	if err != nil || !slices.Equal(held.Dependencies, want) {
		t.Fatalf("transitive graph = %v, %v; want %v", held, err, want)
	}
	retained := plugin.RetainedReleases(nil, held.Dependencies)
	if len(retained) != 3 {
		t.Fatalf("transitive releases were reclaimable: %v", retained)
	}
	// Returned projections cannot change connection-owned bindings.
	held.Dependencies[0] = plugin.Dependency{}
	if again, err := c.activation(root.session); err != nil || !slices.Equal(again.Dependencies, want) {
		t.Fatalf("caller mutated activation graph: %v, %v", again, err)
	}
	original, err := c.activation(root.session)
	if err != nil {
		t.Fatal(err)
	}
	bindings := c.sessions[middle.session].bindings.Snapshot()
	bindings[0].Bindings = append(bindings[0].Bindings, bindings[0].Bindings[0])
	bindings[0].Bindings[1].Contribution = "another-contribution"
	c.sessions[middle.session].bindings, err = plugin.NewRequiredBindings(bindings)
	if err != nil {
		t.Fatal(err)
	}
	expanded, err := c.activation(root.session)
	if err != nil || expanded.Identity == original.Identity || !slices.Equal(expanded.Dependencies, original.Dependencies) {
		t.Fatalf("same-release contribution change was not identified: %+v %v", expanded, err)
	}
	slices.Reverse(bindings[0].Bindings)
	c.sessions[middle.session].bindings, err = plugin.NewRequiredBindings(bindings)
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := c.activation(root.session)
	if err != nil || reordered.Identity == expanded.Identity || !slices.Equal(reordered.Dependencies, expanded.Dependencies) {
		t.Fatalf("same-release ordering change was not identified: %+v %v", reordered, err)
	}
	bind(middle, leaf, d, true)
	if restored, err := c.activation(root.session); err != nil || !restored.Equal(original) {
		t.Fatalf("identical graph did not restore its identity: %+v %v", restored, err)
	}
	oldSession := leaf.session
	leaf.session = &sdk.ClientSession{}
	c.sessions[leaf.session] = &ownedSession{bindings: c.sessions[oldSession].bindings}
	if _, err := c.activation(oldSession); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("retired generation remained usable: %v", err)
	}
	if reconnected, err := c.activation(root.session); err != nil || !reconnected.Equal(original) {
		t.Fatalf("equivalent reconnect changed binding identity: %+v %v", reconnected, err)
	}
	originalSource := leaf.config.Source
	leaf.config.Source, err = mcpserver.InstallationSource(d.InstallationID, testsupport.Digest("replacement"), testsupport.Digest("authority"), testsupport.Digest("recipient"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.activation(root.session); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("replacement release entered the original graph: %v", err)
	}
	leaf.config.Source = originalSource
	leaf.session = nil
	if _, err := c.activation(root.session); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("bound optional failure became absence: %v", err)
	}
}
