package plugins

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type requirementInstallations struct {
	Store
	snapshots []plugin.Snapshot
}

func (s requirementInstallations) List(context.Context) ([]plugin.Snapshot, error) {
	return s.snapshots, nil
}

func (s requirementInstallations) Get(_ context.Context, id identity.InstallationID) (plugin.Snapshot, bool, error) {
	for _, snapshot := range s.snapshots {
		if snapshot.Installation.ID() == id {
			return snapshot, true, nil
		}
	}
	return plugin.Snapshot{}, false, nil
}

type requirementSelections struct {
	SelectionStore
	values []plugin.ProviderSelection
}

func (s requirementSelections) List(context.Context) ([]plugin.ProviderSelection, error) {
	return s.values, nil
}

func requirementContract(t *testing.T, reference string, composition portable.Composition) *portable.Capability {
	t.Helper()
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: reference, Composition: composition, Scope: "installation"}, portable.MethodDescriptor{Name: reference + ".read", Kind: portable.Query, Target: "installation", Input: jsontext.Value(`{"type":"object"}`), Output: jsontext.Value(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func requirementSnapshot(t *testing.T, id identity.InstallationID, name string, contract *portable.Capability, requires ...plugin.Requirement) plugin.Snapshot {
	t.Helper()
	server := testsupport.ServerName("backend")
	release, err := plugin.NewRelease(testsupport.Digest(name+contract.Identity().String()+fmt.Sprint(requires)), plugin.Declaration{Name: name, Servers: []plugin.Server{{Name: server, Transport: mcpserver.TransportStdio, Command: "backend"}}, Contributions: []plugin.Contribution{{ID: "operations", Server: server, Export: "operations", Contract: contract.Identity(), Requires: requires}}})
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(id, "source", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	return plugin.Snapshot{Installation: installation, Selected: release}
}

func TestNamedRequirementsUseSelectedBindingsAndExplicitOptionalAbsence(t *testing.T) {
	rootContract := requirementContract(t, "reviews", portable.Collection)
	selected := requirementContract(t, "model", portable.Selection)
	optional := requirementContract(t, "analyzers", portable.Collection)
	root := requirementSnapshot(t, testsupport.InstallationID(t), "reviews", rootContract, plugin.Requirement{ID: "model", Contract: selected.Identity()}, plugin.Requirement{ID: "analyzers", Contract: optional.Identity(), Optional: true})
	first := requirementSnapshot(t, testsupport.InstallationID(t), "model-one", selected)
	second := requirementSnapshot(t, testsupport.InstallationID(t), "model-two", selected)
	choice, err := plugin.NewProviderSelection(plugin.SelectionRecord{Role: "model", Installation: second.Installation.ID(), Contribution: "operations"})
	if err != nil {
		t.Fatal(err)
	}
	store := requirementInstallations{snapshots: []plugin.Snapshot{root, first, second}}
	values, err := ResolveRequirements(t.Context(), testFramework(t, store), store, requirementSelections{values: []plugin.ProviderSelection{choice}}, &requirementCompositions{}, root.Installation.ID(), root.Selected.Digest(), "operations")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0].ID != "model" || len(values[0].Bindings) != 1 || values[0].Bindings[0].InstallationID != second.Installation.ID() || len(values[1].Bindings) != 0 {
		t.Fatalf("named collaborators: %+v", values)
	}
	if _, err := ResolveRequirements(t.Context(), testFramework(t, store), store, requirementSelections{}, &requirementCompositions{}, root.Installation.ID(), root.Selected.Digest(), "operations"); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("missing required role: %v", err)
	}
}

func TestRequiredAndOptionalPresentEdgesReportTheirCompleteCycle(t *testing.T) {
	a := requirementContract(t, "a", portable.Collection)
	b := requirementContract(t, "b", portable.Collection)
	idA, idB := testsupport.InstallationID(t), testsupport.InstallationID(t)
	first := requirementSnapshot(t, idA, "first", a, plugin.Requirement{ID: "b", Contract: b.Identity()})
	second := requirementSnapshot(t, idB, "second", b, plugin.Requirement{ID: "a", Contract: a.Identity()})
	store := requirementInstallations{snapshots: []plugin.Snapshot{first, second}}
	_, err := ResolveRequirements(t.Context(), testFramework(t, store), store, requirementSelections{}, &requirementCompositions{}, idA, first.Selected.Digest(), "operations")
	if !errors.Is(err, plugin.ErrInvalid) || !strings.Contains(err.Error(), idA.String()+"/backend/backend -> "+idB.String()+"/backend/backend -> "+idA.String()+"/backend/backend") {
		t.Fatalf("required cycle: %v", err)
	}
	first = requirementSnapshot(t, idA, "first", a, plugin.Requirement{ID: "b", Contract: b.Identity(), Optional: true})
	store.snapshots[0] = first
	values, err := ResolveRequirements(t.Context(), testFramework(t, store), store, requirementSelections{}, &requirementCompositions{}, idA, first.Selected.Digest(), "operations")
	if !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("optional present edge hid an execution cycle: %+v, %v", values, err)
	}
	store.snapshots = []plugin.Snapshot{first}
	values, err = ResolveRequirements(t.Context(), testFramework(t, store), store, requirementSelections{}, &requirementCompositions{}, idA, first.Selected.Digest(), "operations")
	if err != nil || len(values) != 1 || len(values[0].Bindings) != 0 {
		t.Fatalf("explicit optional absence: %+v %v", values, err)
	}
}

func TestRequirementsValidateOtherContributionsSharingTheEntry(t *testing.T) {
	contract := requirementContract(t, "root", portable.Collection)
	missing := requirementContract(t, "missing", portable.Selection)
	root := requirementSnapshot(t, testsupport.InstallationID(t), "root", contract)
	declaration := root.Selected.Declaration()
	declaration.Contributions = append(declaration.Contributions, plugin.Contribution{ID: "sibling", Server: testsupport.ServerName("backend"), Export: "sibling", Contract: contract.Identity(), Requires: []plugin.Requirement{{ID: "missing", Contract: missing.Identity()}}})
	release, err := plugin.NewRelease(root.Selected.Digest(), declaration)
	if err != nil {
		t.Fatal(err)
	}
	root.Selected = release
	store := requirementInstallations{snapshots: []plugin.Snapshot{root}}
	_, err = ResolveRequirements(t.Context(), testFramework(t, store), store, requirementSelections{}, &requirementCompositions{}, root.Installation.ID(), release.Digest(), "operations")
	if !errors.Is(err, plugin.ErrUnavailable) || !strings.Contains(err.Error(), "required dependency missing") {
		t.Fatalf("sibling entry dependency: %v", err)
	}
}

func TestTransitiveRequirementFailureExplainsItsConsumerPath(t *testing.T) {
	rootContract := requirementContract(t, "root", portable.Collection)
	providerContract := requirementContract(t, "provider", portable.Collection)
	missing := requirementContract(t, "missing", portable.Selection)
	root := requirementSnapshot(t, testsupport.InstallationID(t), "root", rootContract, plugin.Requirement{ID: "provider", Contract: providerContract.Identity()})
	provider := requirementSnapshot(t, testsupport.InstallationID(t), "provider", providerContract, plugin.Requirement{ID: "missing", Contract: missing.Identity()})
	store := requirementInstallations{snapshots: []plugin.Snapshot{root, provider}}
	_, err := ResolveRequirements(t.Context(), testFramework(t, store), store, requirementSelections{}, &requirementCompositions{}, root.Installation.ID(), root.Selected.Digest(), "operations")
	if !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("transitive absence lost its category: %v", err)
	}
	for _, owner := range []string{root.Installation.ID().String() + "/backend/backend", provider.Installation.ID().String() + "/backend/backend", "requires operations/provider", "requires operations/missing"} {
		if !strings.Contains(err.Error(), owner) {
			t.Errorf("dependency failure omits %q: %v", owner, err)
		}
	}
}

func TestEntryRequirementsFreezeTheSelectedProviderForTheUseInterval(t *testing.T) {
	contract := requirementContract(t, "consumer", portable.Collection)
	model := requirementContract(t, "model", portable.Selection)
	root := requirementSnapshot(t, testsupport.InstallationID(t), "consumer", contract, plugin.Requirement{ID: "model", Contract: model.Identity()})
	first := requirementSnapshot(t, testsupport.InstallationID(t), "first", model)
	second := requirementSnapshot(t, testsupport.InstallationID(t), "second", model)
	choice, err := plugin.NewProviderSelection(plugin.SelectionRecord{Role: "model", Installation: first.Installation.ID(), Contribution: "operations"})
	if err != nil {
		t.Fatal(err)
	}
	store := requirementInstallations{snapshots: []plugin.Snapshot{root, first, second}}
	selections := requirementSelections{values: []plugin.ProviderSelection{choice}}
	bound, err := resolveEntryRequirements(t.Context(), testFramework(t, store), store, selections, &requirementCompositions{}, root, testsupport.ServerName("backend"))
	if err != nil {
		t.Fatal(err)
	}
	selections.values[0], err = plugin.NewProviderSelection(plugin.SelectionRecord{Role: "model", Installation: second.Installation.ID(), Contribution: "operations"})
	if err != nil {
		t.Fatal(err)
	}
	future, err := resolveEntryRequirements(t.Context(), testFramework(t, store), store, selections, &requirementCompositions{}, root, testsupport.ServerName("backend"))
	if err != nil {
		t.Fatal(err)
	}
	original := plugin.Dependency{InstallationID: first.Installation.ID(), Digest: first.Selected.Digest()}
	replacement := plugin.Dependency{InstallationID: second.Installation.ID(), Digest: second.Selected.Digest()}
	if !bound.Allows(original, "operations", model.Identity()) || bound.Allows(replacement, "operations", model.Identity()) || !future.Allows(replacement, "operations", model.Identity()) {
		t.Fatal("selection change retargeted an existing dependency projection")
	}
}
