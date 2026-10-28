package plugins

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/qimu"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
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

func ResolveRequirements(ctx context.Context, host *qimu.Host, store Store, selections SelectionStore, compositions CompositionStore, installation identity.InstallationID, digest identity.Digest, contribution plugin.ContributionID) ([]Collaborator, error) {
	if host == nil {
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
		contract, err := portable.ParseCapability(record.Contract.JSON())
		if err != nil {
			return requirementCatalog{}, err
		}
		catalog.compositions[contract.Descriptor().Reference] = record
	}
	return catalog, nil
}

func resolveEntryRequirements(ctx context.Context, host *qimu.Host, store Store, selections SelectionStore, compositions CompositionStore, root plugin.Snapshot, server mcpserver.ServerName) (*plugin.RequiredBindings, error) {
	if host == nil {
		return nil, plugin.ErrInvalid
	}
	catalog, err := readRequirementCatalog(ctx, store, selections, compositions)
	if err != nil {
		return nil, err
	}
	current, found := catalog.installations[root.Installation.ID()]
	if !found || current.Selected.Digest() != root.Selected.Digest() {
		return nil, plugin.ErrStale
	}
	if !current.Installation.ServerEnabled(server) {
		return nil, plugin.ErrDisabled
	}
	plan, err := catalog.plan(ctx, host, current, server)
	if err != nil {
		return nil, err
	}
	binding, err := plan.Snapshot()
	if err != nil {
		return nil, err
	}
	return catalog.projectRequirements(current, server, binding)
}

// ProjectRequiredBindings projects already captured providers for the profile's
// setup wire. It never consults a role selection or substitutes a provider.
func ProjectRequiredBindings(ctx context.Context, store Store, root plugin.Snapshot, server mcpserver.ServerName, binding qimu.BindingSnapshot) (*plugin.RequiredBindings, error) {
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

func (c requirementCatalog) projectRequirements(current plugin.Snapshot, server mcpserver.ServerName, binding qimu.BindingSnapshot) (*plugin.RequiredBindings, error) {
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

func bindingKey(binding ContributionBinding) string {
	return binding.InstallationID.String() + "/" + binding.ContributionID.String()
}

func (c requirementCatalog) candidates(required plugin.Requirement) ([]ContributionBinding, error) {
	contract, err := portable.ParseCapability(required.Contract.JSON())
	if err != nil {
		return nil, err
	}
	descriptor := contract.Descriptor()
	var bindings []ContributionBinding
	if descriptor.Composition == portable.Decoration {
		record, found := c.compositions[descriptor.Reference]
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
	for _, snapshot := range c.installations {
		for _, declaration := range snapshot.Selected.Declaration().Contributions {
			if declaration.Contract != contract.Identity() || !snapshot.Installation.ServerEnabled(declaration.Server) {
				continue
			}
			if descriptor.Composition == portable.Selection {
				choice, found := c.selections[descriptor.Reference]
				if !found || choice.Installation != snapshot.Installation.ID() || choice.Contribution != declaration.ID {
					continue
				}
			}
			binding, err := c.binding(snapshot, declaration)
			if err != nil {
				return nil, err
			}
			bindings = append(bindings, binding)
		}
	}
	slices.SortFunc(bindings, func(a, b ContributionBinding) int { return strings.Compare(bindingKey(a), bindingKey(b)) })
	if len(bindings) == 0 && !required.Optional {
		return nil, fmt.Errorf("%w: required dependency %s (%s) has no enabled implementation", plugin.ErrUnavailable, required.ID, descriptor.Reference)
	}
	return bindings, nil
}

func (c requirementCatalog) collaborators(root plugin.Snapshot, declaration plugin.Contribution, binding qimu.BindingSnapshot) ([]Collaborator, error) {
	resolved := make([]Collaborator, 0, len(declaration.Requires))
	for _, required := range declaration.Requires {
		var chosen *qimu.ImportDeclaration
		for _, unit := range binding.Units {
			if unit.Source.InstallationID != qimu.InstallationID(root.Installation.ID().String()) || unit.Source.Release != qimu.Digest(root.Selected.Digest().String()) || unit.Source.UnitID != BackendUnit(declaration.Server) {
				continue
			}
			for _, slot := range unit.Imports {
				if slot.Slot == RequirementSlot(declaration.ID, required.ID) {
					copy := slot
					chosen = &copy
					break
				}
			}
		}
		if chosen == nil {
			return nil, fmt.Errorf("%w: captured requirement is missing", plugin.ErrInvalid)
		}
		bindings := make([]ContributionBinding, 0, len(chosen.Providers))
		for _, provider := range chosen.Providers {
			id, err := identity.ParseInstallation(provider.InstallationID.String())
			if err != nil {
				return nil, err
			}
			snapshot, found := c.installations[id]
			if !found || snapshot.Selected.Digest().String() != provider.Release.String() {
				return nil, plugin.ErrStale
			}
			contribution, found := snapshot.Selected.Contribution(plugin.ContributionID(provider.Export))
			if !found || BackendUnit(contribution.Server) != provider.UnitID || contribution.Contract != required.Contract {
				return nil, plugin.ErrUnavailable
			}
			actual, err := c.binding(snapshot, contribution)
			if err != nil {
				return nil, err
			}
			source, err := ContributionSource(actual)
			if err != nil || source != provider {
				return nil, fmt.Errorf("%w: captured provider contract differs", plugin.ErrInvalid)
			}
			bindings = append(bindings, actual)
		}
		resolved = append(resolved, Collaborator{ID: required.ID, Bindings: bindings})
	}
	return resolved, nil
}

func (c requirementCatalog) plan(ctx context.Context, host *qimu.Host, root plugin.Snapshot, server mcpserver.ServerName) (*qimu.BindingPlan, error) {
	choices := []qimu.BindingChoice{}
	for _, snapshot := range c.installations {
		if !snapshot.Installation.Enabled() {
			continue
		}
		for _, contribution := range snapshot.Selected.Declaration().Contributions {
			for _, required := range contribution.Requires {
				choice := qimu.BindingChoice{Unit: qimu.UnitRef{InstallationID: qimu.InstallationID(snapshot.Installation.ID().String()), Release: qimu.Digest(snapshot.Selected.Digest().String()), UnitID: BackendUnit(contribution.Server)}, Slot: RequirementSlot(contribution.ID, required.ID), Providers: []qimu.ExportRef{}}
				providers, err := c.candidates(required)
				if err != nil {
					choice.Providers = nil
					choice.Failure = err
				} else {
					for _, provider := range providers {
						source, err := ContributionSource(provider)
						if err != nil {
							return nil, err
						}
						choice.Providers = append(choice.Providers, source)
					}
				}
				choices = append(choices, choice)
			}
		}
	}
	unit := qimu.UnitRef{InstallationID: qimu.InstallationID(root.Installation.ID().String()), Release: qimu.Digest(root.Selected.Digest().String()), UnitID: BackendUnit(server)}
	plan, err := host.PlanUnit(ctx, "", unit, choices)
	return plan, FrameworkError(err)
}
