package plugins

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
)

// Collaborator is an explicit absence or a fixed set of named consumer bindings.
// It is prepared from declarations without starting a backend.

type Collaborator struct {
	ID       plugin.ContributionID
	Bindings []ContributionBinding
}

type requirementCatalog struct {
	installations map[identity.InstallationID]plugin.Snapshot
	selections    map[string]plugin.SelectionRecord
	compositions  map[string]plugin.CompositionRecord
}

func ResolveRequirements(ctx context.Context, host framehost.Host, store Store, selections SelectionStore, compositions CompositionStore, installation identity.InstallationID, digest framevalue.Digest, contribution plugin.ContributionID) ([]Collaborator, error) {
	if !host.Valid() {
		return nil, plugin.ErrInvalid
	}
	catalog, err := readRequirementCatalog(ctx, store, selections, compositions)
	if err != nil {
		return nil, err
	}
	root, found := catalog.installations[installation]
	if !found {
		return nil, plugin.ErrNotFound
	}
	if root.Selected.Digest() != digest {
		return nil, plugin.ErrStale
	}
	declared, found := root.Selected.Contribution(contribution)
	if !found {
		return nil, plugin.ErrNotFound
	}
	if !root.Installation.ServerEnabled(declared.Server) {
		return nil, plugin.ErrDisabled
	}
	plan, err := catalog.plan(ctx, host, root, declared.Server)
	if err != nil {
		return nil, err
	}
	binding, err := plan.Snapshot()
	if err != nil {
		return nil, err
	}
	return catalog.collaborators(root, declared, binding)
}

func readRequirementCatalog(ctx context.Context, store Store, selections SelectionStore, compositions CompositionStore) (requirementCatalog, error) {
	snapshots, err := store.List(ctx)
	if err != nil {
		return requirementCatalog{}, err
	}
	choices, err := selections.List(ctx)
	if err != nil {
		return requirementCatalog{}, err
	}
	ordered, err := compositions.List(ctx)
	if err != nil {
		return requirementCatalog{}, err
	}
	catalog := requirementCatalog{installations: make(map[identity.InstallationID]plugin.Snapshot, len(snapshots)), selections: make(map[string]plugin.SelectionRecord, len(choices)), compositions: make(map[string]plugin.CompositionRecord, len(ordered))}
	for _, snapshot := range snapshots {
		catalog.installations[snapshot.Installation.ID()] = snapshot
	}
	for _, choice := range choices {
		record := choice.Snapshot()
		catalog.selections[record.Role] = record
	}
	for _, value := range ordered {
		record := value.Snapshot()
		contract, err := plugincontract.ParseCapability(record.Contract.Bytes())
		if err != nil {
			return requirementCatalog{}, err
		}
		catalog.compositions[contract.Descriptor().Reference] = record
	}
	return catalog, nil
}

// ProjectRequiredBindings projects already captured providers for the profile's
// setup wire. It never consults a role selection or substitutes a provider.
func ProjectRequiredBindings(ctx context.Context, store Store, root plugin.Snapshot, server mcpserver.ServerName, binding framehost.BindingSnapshot) (*plugin.RequiredBindings, error) {
	snapshots, err := store.List(ctx)
	if err != nil {
		return nil, err
	}
	catalog := requirementCatalog{installations: make(map[identity.InstallationID]plugin.Snapshot, len(snapshots))}
	for _, snapshot := range snapshots {
		catalog.installations[snapshot.Installation.ID()] = snapshot
	}
	return catalog.projectRequirements(root, server, binding)
}

func (c requirementCatalog) projectRequirements(current plugin.Snapshot, server mcpserver.ServerName, binding framehost.BindingSnapshot) (*plugin.RequiredBindings, error) {
	var values []plugin.ResolvedRequirement
	for _, contribution := range current.Selected.Declaration().Contributions {
		if contribution.Server != server {
			continue
		}
		collaborators, err := c.collaborators(current, contribution, binding)
		if err != nil {
			return nil, err
		}
		for index, required := range contribution.Requires {
			bindings := collaborators[index].Bindings
			value := plugin.ResolvedRequirement{Contribution: contribution.ID, ID: required.ID, Contract: required.Contract, Optional: required.Optional}
			for _, binding := range bindings {
				value.Bindings = append(value.Bindings, plugin.BoundContribution{Provider: plugin.Dependency{InstallationID: binding.InstallationID, Digest: binding.Digest}, Contribution: binding.ContributionID, Server: binding.ServerName, Export: binding.Export, Contract: binding.Contract})
			}
			values = append(values, value)
		}
	}
	return plugin.NewRequiredBindings(values)
}

func (c requirementCatalog) binding(snapshot plugin.Snapshot, declaration plugin.Contribution) (ContributionBinding, error) {
	id, err := snapshot.Installation.ServerID(declaration.Server)
	if err != nil {
		return ContributionBinding{}, err
	}
	return ContributionBinding{InstallationID: snapshot.Installation.ID(), ContributionID: declaration.ID, Digest: snapshot.Selected.Digest(), ServerID: id, ServerName: declaration.Server, Export: declaration.Export, Contract: declaration.Contract}, nil
}

// compareBindings orders bindings by installation, then contribution.
func compareBindings(a, b ContributionBinding) int {
	return cmp.Or(
		strings.Compare(a.InstallationID.String(), b.InstallationID.String()),
		strings.Compare(a.ContributionID.String(), b.ContributionID.String()),
	)
}

func (c requirementCatalog) candidates(required plugin.Requirement) ([]ContributionBinding, error) {
	contract, err := plugincontract.ParseCapability(required.Contract.Bytes())
	if err != nil {
		return nil, err
	}
	descriptor := contract.Descriptor()
	if descriptor.Composition == plugincontract.Decoration {
		return c.decorators(required, contract)
	}
	var bindings []ContributionBinding
	for _, snapshot := range c.installations {
		for _, declaration := range snapshot.Selected.Declaration().Contributions {
			if !c.implements(snapshot, declaration, contract) {
				continue
			}
			binding, err := c.binding(snapshot, declaration)
			if err != nil {
				return nil, err
			}
			bindings = append(bindings, binding)
		}
	}
	slices.SortFunc(bindings, compareBindings)
	if len(bindings) == 0 && !required.Optional {
		return nil, fmt.Errorf("%w: required dependency %s (%s) has no enabled implementation", plugin.ErrUnavailable, required.ID, descriptor.Reference)
	}
	return bindings, nil
}

// implements reports whether an enabled contribution serves the contract; a
// selection contract admits only the contribution the user selected.
func (c requirementCatalog) implements(snapshot plugin.Snapshot, declaration plugin.Contribution, contract *plugincontract.Capability) bool {
	if declaration.Contract != contract.Identity() || !snapshot.Installation.ServerEnabled(declaration.Server) {
		return false
	}
	descriptor := contract.Descriptor()
	if descriptor.Composition != plugincontract.Selection {
		return true
	}
	choice, found := c.selections[descriptor.Reference]
	return found && choice.Installation == snapshot.Installation.ID() && choice.Contribution == declaration.ID
}

// decorators resolves a decoration contract through its explicit ordered
// composition; decorators never apply by mere installation.
func (c requirementCatalog) decorators(required plugin.Requirement, contract *plugincontract.Capability) ([]ContributionBinding, error) {
	record, found := c.compositions[contract.Descriptor().Reference]
	if !found {
		if required.Optional {
			return []ContributionBinding{}, nil
		}
		return nil, fmt.Errorf("%w: dependency %s needs an explicit decoration composition", plugin.ErrUnavailable, required.ID)
	}
	if record.Contract != contract.Identity() {
		return nil, fmt.Errorf("%w: dependency %s composition contract differs", plugin.ErrUnavailable, required.ID)
	}
	composition, err := plugin.NewOrderedComposition(record)
	if err != nil {
		return nil, err
	}
	bindings, err := resolveCompositionMembers(composition, func(id identity.InstallationID) (plugin.Snapshot, bool, error) {
		snapshot, found := c.installations[id]
		return snapshot, found, nil
	})
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 && !required.Optional {
		return nil, fmt.Errorf("%w: required dependency %s composition is empty", plugin.ErrUnavailable, required.ID)
	}
	return bindings, nil
}

func (c requirementCatalog) collaborators(root plugin.Snapshot, declaration plugin.Contribution, binding framehost.BindingSnapshot) ([]Collaborator, error) {
	resolved := make([]Collaborator, 0, len(declaration.Requires))
	for _, required := range declaration.Requires {
		chosen, found := capturedRequirement(root, declaration, required, binding)
		if !found {
			return nil, fmt.Errorf("%w: captured requirement is missing", plugin.ErrInvalid)
		}
		bindings := make([]ContributionBinding, 0, len(chosen.Providers))
		for _, provider := range chosen.Providers {
			actual, err := c.capturedProvider(provider, required)
			if err != nil {
				return nil, err
			}
			bindings = append(bindings, actual)
		}
		resolved = append(resolved, Collaborator{ID: required.ID, Bindings: bindings})
	}
	return resolved, nil
}

// capturedRequirement finds the import slot the captured binding resolved for
// one requirement of the root's own backend unit.
func capturedRequirement(root plugin.Snapshot, declaration plugin.Contribution, required plugin.Requirement, binding framehost.BindingSnapshot) (framehost.ImportBinding, bool) {
	var chosen framehost.ImportBinding
	found := false
	for _, unit := range binding.Units {
		if unit.Source.InstallationID != root.Installation.ID().Host() || unit.Source.Release != root.Selected.Digest() || unit.Source.UnitID != BackendUnit(declaration.Server) {
			continue
		}
		for _, slot := range unit.Imports {
			if slot.Slot == RequirementSlot(declaration.ID, required.ID) {
				chosen, found = slot, true
				break
			}
		}
	}
	return chosen, found
}

// capturedProvider rebinds a captured provider only while its installation
// still selects the same release and contract.
func (c requirementCatalog) capturedProvider(provider framehost.ExportRef, required plugin.Requirement) (ContributionBinding, error) {
	id, err := identity.ParseHostInstallation(provider.InstallationID)
	if err != nil {
		return ContributionBinding{}, err
	}
	snapshot, found := c.installations[id]
	if !found || snapshot.Selected.Digest() != provider.Release {
		return ContributionBinding{}, plugin.ErrStale
	}
	contribution, found := snapshot.Selected.Contribution(plugin.ContributionID(provider.Export))
	if !found || BackendUnit(contribution.Server) != provider.UnitID || contribution.Contract != required.Contract {
		return ContributionBinding{}, plugin.ErrUnavailable
	}
	actual, err := c.binding(snapshot, contribution)
	if err != nil {
		return ContributionBinding{}, err
	}
	source, err := ContributionSource(actual)
	if err != nil || source != provider {
		return ContributionBinding{}, fmt.Errorf("%w: captured provider contract differs", plugin.ErrInvalid)
	}
	return actual, nil
}

func (c requirementCatalog) plan(ctx context.Context, host framehost.Host, root plugin.Snapshot, server mcpserver.ServerName) (framehost.BindingPlan, error) {
	unit := framehost.UnitRef{InstallationID: root.Installation.ID().Host(), Release: root.Selected.Digest(), UnitID: BackendUnit(server)}
	type entry struct {
		source framehost.UnitRef
		path   string
	}
	pending := []entry{{source: unit}}
	visited := map[framehost.UnitRef]bool{}
	choices := []framehost.BindingChoice{}
	for len(pending) > 0 {
		next := pending[0]
		pending = pending[1:]
		current := next.source
		if visited[current] {
			continue
		}
		visited[current] = true
		id, err := identity.ParseHostInstallation(current.InstallationID)
		if err != nil {
			return framehost.BindingPlan{}, err
		}
		snapshot, found := c.installations[id]
		if !found || !snapshot.Installation.Enabled() || snapshot.Selected.Digest() != current.Release {
			return framehost.BindingPlan{}, fmt.Errorf("%s%s: %w", next.path, current, plugin.ErrUnavailable)
		}
		for _, contribution := range snapshot.Selected.Declaration().Contributions {
			if BackendUnit(contribution.Server) != current.UnitID {
				continue
			}
			for _, required := range contribution.Requires {
				choice := framehost.BindingChoice{Unit: current, Slot: RequirementSlot(contribution.ID, required.ID), Providers: []framehost.ExportRef{}}
				path := next.path + current.String() + " requires " + choice.Slot
				providers, err := c.candidates(required)
				if err != nil {
					return framehost.BindingPlan{}, fmt.Errorf("%s: %w", path, err)
				}
				for _, provider := range providers {
					source, err := ContributionSource(provider)
					if err != nil {
						return framehost.BindingPlan{}, err
					}
					choice.Providers = append(choice.Providers, source)
					pending = append(pending, entry{source: source.Unit(), path: path + ": "})
				}
				choices = append(choices, choice)
			}
		}
	}
	plan, err := host.PlanUnit(ctx, "", unit, choices)
	return plan, FrameworkError(err)
}
