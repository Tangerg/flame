package plugins

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
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
}

func ResolveRequirements(ctx context.Context, store Store, selections SelectionStore, installation resourceid.InstallationID, digest fingerprint.Digest, contribution plugin.ContributionID) ([]Collaborator, error) {
	snapshots, err := store.List(ctx)
	if err != nil {
		return nil, err
	}
	choices, err := selections.List(ctx)
	if err != nil {
		return nil, err
	}
	catalog := requirementCatalog{installations: make(map[resourceid.InstallationID]plugin.Snapshot, len(snapshots)), selections: make(map[string]plugin.SelectionRecord, len(choices))}
	for _, snapshot := range snapshots {
		catalog.installations[snapshot.Installation.ID()] = snapshot
	}
	for _, choice := range choices {
		record := choice.Snapshot()
		catalog.selections[record.Role] = record
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
		return nil, fmt.Errorf("%w: dependency %s needs an explicit decoration composition", plugin.ErrUnavailable, required.ID)
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
				return err
			}
			for _, binding := range bindings {
				if binding.InstallationID == snapshot.Installation.ID() && binding.ServerName.String() == server {
					continue
				}
				provider := c.installations[binding.InstallationID]
				if err := c.validateEntry(provider, binding.ServerName.String(), path); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
