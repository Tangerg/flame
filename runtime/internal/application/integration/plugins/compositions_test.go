package plugins

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"github.com/Tangerg/flame/qimu"
	"testing"

	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type requirementCompositions struct {
	CompositionStore
	values []plugin.OrderedComposition
}

func (s requirementCompositions) List(context.Context) ([]plugin.OrderedComposition, error) {
	return s.values, nil
}

func (s requirementCompositions) Get(_ context.Context, reference string) (plugin.OrderedComposition, bool, error) {
	for _, value := range s.values {
		contract, err := portable.ParseCapability(value.Snapshot().Contract.JSON())
		if err != nil {
			return plugin.OrderedComposition{}, false, err
		}
		if contract.Descriptor().Reference == reference {
			return value, true, nil
		}
	}
	return plugin.OrderedComposition{}, false, nil
}

func (s *requirementCompositions) Save(_ context.Context, value plugin.OrderedComposition) error {
	s.values = []plugin.OrderedComposition{value}
	return nil
}

func TestOrderedCompositionFreezesExplicitOrderAndRejectsWithdrawnMembers(t *testing.T) {
	contract := requirementContract(t, "model-observer", portable.Decoration)
	first := requirementSnapshot(t, testsupport.InstallationID(t), "outer", contract)
	second := requirementSnapshot(t, testsupport.InstallationID(t), "inner", contract)
	store := requirementInstallations{snapshots: []plugin.Snapshot{first, second}}
	choices := &requirementCompositions{}
	owner, err := NewCompositions(choices, store, qimu.NewAdmission(), nil)
	if err != nil {
		t.Fatal(err)
	}
	outer := plugin.CompositionMember{Installation: first.Installation.ID(), Contribution: "operations"}
	inner := plugin.CompositionMember{Installation: second.Installation.ID(), Contribution: "operations"}
	value, err := owner.Set(t.Context(), plugin.CompositionRecord{Contract: contract.Identity(), Members: []plugin.CompositionMember{inner, outer}})
	if err != nil {
		t.Fatal(err)
	}
	projection := value.Snapshot()
	projection.Members[0] = outer
	bound, err := owner.Resolve(t.Context(), contract)
	if err != nil || len(bound) != 2 || bound[0].InstallationID != inner.Installation || bound[1].InstallationID != outer.Installation {
		t.Fatalf("explicit order changed: %+v, %v", bound, err)
	}
	if _, err := owner.Set(t.Context(), plugin.CompositionRecord{Contract: contract.Identity(), Members: []plugin.CompositionMember{outer, inner}}); err != nil {
		t.Fatal(err)
	}
	if bound[0].InstallationID != inner.Installation {
		t.Fatal("changing configuration retargeted a frozen chain")
	}
	if _, err := owner.Set(t.Context(), plugin.CompositionRecord{Contract: contract.Identity(), Members: []plugin.CompositionMember{outer, outer}}); !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("duplicate member accepted: %v", err)
	}
	descriptor := contract.Descriptor()
	descriptor.Methods[0].Output = jsontext.Value(`{"type":"boolean"}`)
	drifted, err := portable.AdmitCapability(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Resolve(t.Context(), drifted); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("changed consumer contract adopted stored composition: %v", err)
	}
	second.Installation.Disable()
	if _, err := owner.Resolve(t.Context(), contract); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("withdrawn member silently skipped: %v", err)
	}
}

func TestNamedDecorationRequirementsPreserveStoredOrderAndRequiredAbsence(t *testing.T) {
	contract := requirementContract(t, "observed-model", portable.Decoration)
	rootContract := requirementContract(t, "consumer", portable.Collection)
	root := requirementSnapshot(t, testsupport.InstallationID(t), "consumer", rootContract, plugin.Requirement{ID: "observers", Contract: contract.Identity()})
	first := requirementSnapshot(t, testsupport.InstallationID(t), "outer", contract)
	second := requirementSnapshot(t, testsupport.InstallationID(t), "inner", contract)
	composition, err := plugin.NewOrderedComposition(plugin.CompositionRecord{Contract: contract.Identity(), Members: []plugin.CompositionMember{{Installation: second.Installation.ID(), Contribution: "operations"}, {Installation: first.Installation.ID(), Contribution: "operations"}}})
	if err != nil {
		t.Fatal(err)
	}
	store := requirementInstallations{snapshots: []plugin.Snapshot{root, first, second}}
	choices := &requirementCompositions{values: []plugin.OrderedComposition{composition}}
	bound, err := resolveEntryRequirements(t.Context(), testFramework(t, store), store, requirementSelections{}, choices, root, testsupport.ServerName("backend"))
	if err != nil {
		t.Fatal(err)
	}
	values := bound.Snapshot()
	if len(values) != 1 || len(values[0].Bindings) != 2 || values[0].Bindings[0].Provider.InstallationID != second.Installation.ID() {
		t.Fatalf("setup order lost: %+v", values)
	}
	if _, err := resolveEntryRequirements(t.Context(), testFramework(t, store), store, requirementSelections{}, &requirementCompositions{}, root, testsupport.ServerName("backend")); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("required absence admitted: %v", err)
	}
	empty, err := plugin.NewOrderedComposition(plugin.CompositionRecord{Contract: contract.Identity(), Members: []plugin.CompositionMember{}})
	if err != nil {
		t.Fatal(err)
	}
	choices.values = []plugin.OrderedComposition{empty}
	if _, err := resolveEntryRequirements(t.Context(), testFramework(t, store), store, requirementSelections{}, choices, root, testsupport.ServerName("backend")); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("required empty composition admitted: %v", err)
	}
	optional := requirementSnapshot(t, testsupport.InstallationID(t), "optional", rootContract, plugin.Requirement{ID: "observers", Contract: contract.Identity(), Optional: true})
	store.snapshots = append(store.snapshots, optional)
	if value, err := resolveEntryRequirements(t.Context(), testFramework(t, store), store, requirementSelections{}, choices, optional, testsupport.ServerName("backend")); err != nil || len(value.Snapshot()[0].Bindings) != 0 {
		t.Fatalf("optional empty composition: %+v, %v", value, err)
	}
	changed := requirementContract(t, "observed-model", portable.Selection)
	if _, err := ResolveComposition(t.Context(), store, choices, changed); !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("non-decoration consumer admitted: %v", err)
	}
}
