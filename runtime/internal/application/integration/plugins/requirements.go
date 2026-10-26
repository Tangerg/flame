package plugins

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/fingerprint"
	"github.com/Tangerg/flame/packages/go-sdk/resourceid"
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
	installations map[resourceid.InstallationID]plugin.Snapshot
	selections    map[string]plugin.SelectionRecord
	compositions  map[string]plugin.CompositionRecord
}

func ResolveRequirements(ctx context.Context, store Store, selections SelectionStore, compositions CompositionStore, installation resourceid.InstallationID, digest fingerprint.Digest, contribution plugin.ContributionID) ([]Collaborator, error) {
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
	if err := catalog.validateEntry(root, declared.Server.String(), nil); err != nil {
		return nil, err
	}
	return catalog.collaborators(declared)
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
	catalog := requirementCatalog{installations: make(map[resourceid.InstallationID]plugin.Snapshot, len(snapshots)), selections: make(map[string]plugin.SelectionRecord, len(choices)), compositions: make(map[string]plugin.CompositionRecord, len(ordered))}
	for _, snapshot := range snapshots {
		catalog.installations[snapshot.Installation.ID()] = snapshot
	}
	for _, choice := range choices {
		record := choice.Snapshot()
		catalog.selections[record.Role] = record
	}
	for _, value := range ordered {
		record := value.Snapshot()
		contract, err := capability.Parse(record.Contract.JSON())
		if err != nil {
			return requirementCatalog{}, err
		}
		catalog.compositions[contract.Descriptor().Reference] = record
	}
	return catalog, nil
}

func ResolveEntryRequirements(ctx context.Context, store Store, selections SelectionStore, compositions CompositionStore, root plugin.Snapshot, server mcpserver.ServerName) (*plugin.RequiredBindings, error) {
	declarations := root.Selected.Declaration().Contributions
	hasRequirements := slices.ContainsFunc(declarations, func(value plugin.Contribution) bool { return value.Server == server && len(value.Requires) > 0 })
	if !hasRequirements {
		return plugin.NewRequiredBindings(nil)
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
	if err := catalog.validateEntry(current, server.String(), nil); err != nil {
		return nil, err
	}
	var values []plugin.ResolvedRequirement
	for _, contribution := range current.Selected.Declaration().Contributions {
		if contribution.Server != server {
			continue
		}
		for _, required := range contribution.Requires {
			bindings, err := catalog.candidates(required)
			if err != nil {
				return nil, err
			}
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
	contract, err := capability.Parse(required.Contract.JSON())
	if err != nil {
		return nil, err
	}
	descriptor := contract.Descriptor()
	var bindings []ContributionBinding
	if descriptor.Composition == capability.Decoration {
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
		bindings, err := resolveCompositionMembers(composition, func(id resourceid.InstallationID) (plugin.Snapshot, bool, error) {
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
			if descriptor.Composition == capability.Selection {
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

func (c requirementCatalog) collaborators(declaration plugin.Contribution) ([]Collaborator, error) {
	resolved := make([]Collaborator, 0, len(declaration.Requires))
	for _, required := range declaration.Requires {
		bindings, err := c.candidates(required)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, Collaborator{ID: required.ID, Bindings: bindings})
	}
	return resolved, nil
}

func (c requirementCatalog) validateEntry(snapshot plugin.Snapshot, server string, path []string) error {
	key := snapshot.Installation.ID().String() + "/" + server
	if slices.Contains(path, key) {
		return fmt.Errorf("%w: required dependency cycle: %s", plugin.ErrInvalid, strings.Join(append(slices.Clone(path), key), " -> "))
	}
	path = append(slices.Clone(path), key)
	for _, declaration := range snapshot.Selected.Declaration().Contributions {
		if declaration.Server.String() != server {
			continue
		}
		for _, required := range declaration.Requires {
			if required.Optional {
				continue
			}
			bindings, err := c.candidates(required)
			if err != nil {
				return fmt.Errorf("plugin dependency %s/%s requires %s: %w", key, declaration.ID, required.ID, err)
			}
			for _, binding := range bindings {
				if binding.InstallationID == snapshot.Installation.ID() && binding.ServerName.String() == server {
					continue
				}
				provider := c.installations[binding.InstallationID]
				if err := c.validateEntry(provider, binding.ServerName.String(), path); err != nil {
					return fmt.Errorf("plugin dependency %s/%s requires %s: %w", key, declaration.ID, required.ID, err)
				}
			}
		}
	}
	return nil
}
