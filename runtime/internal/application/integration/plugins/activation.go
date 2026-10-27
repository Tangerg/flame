package plugins

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// ProjectActivation keeps the complete original graph with its business owner.
// Identity and dependency lists are read projections of that one frozen value.
func ProjectActivation(binding qimu.BindingSnapshot) (plugin.Activation, error) {
	data, err := binding.CanonicalJSON()
	if err != nil {
		return plugin.Activation{}, err
	}
	value, err := jsonvalue.Parse(data)
	if err != nil {
		return plugin.Activation{}, err
	}
	digest, err := identity.ParseDigest(qimu.Hash(data).String())
	if err != nil {
		return plugin.Activation{}, err
	}
	activation := plugin.Activation{Binding: value, Identity: digest}
	for _, dependency := range binding.Dependencies() {
		installation, err := identity.ParseInstallation(dependency.InstallationID.String())
		if err != nil {
			return plugin.Activation{}, err
		}
		release, err := identity.ParseDigest(dependency.Release.String())
		if err != nil {
			return plugin.Activation{}, err
		}
		activation.Dependencies = append(activation.Dependencies, plugin.Dependency{InstallationID: installation, Digest: release})
	}
	return activation, activation.Validate()
}

// DecodeActivation verifies every projection against the original graph. A
// caller cannot authorize a retained route by changing its digest or references.
func DecodeActivation(activation plugin.Activation) (qimu.BindingSnapshot, error) {
	if err := activation.Validate(); err != nil {
		return qimu.BindingSnapshot{}, err
	}
	binding, err := qimu.ParseBinding(activation.Binding.JSON())
	if err != nil {
		return qimu.BindingSnapshot{}, err
	}
	original, err := ProjectActivation(binding)
	if err != nil {
		return qimu.BindingSnapshot{}, err
	}
	if original.Binding != activation.Binding || original.Identity != activation.Identity || !slices.Equal(original.Dependencies, activation.Dependencies) {
		return qimu.BindingSnapshot{}, fmt.Errorf("%w: activation projection differs from its original binding", plugin.ErrInvalid)
	}
	return binding, nil
}

// ReadActivation is the storage boundary for an original retained graph. No
// dependency or identity column can supply an alternative routing fact.
func ReadActivation(data []byte) (plugin.Activation, error) {
	binding, err := qimu.ParseBinding(data)
	if err != nil {
		return plugin.Activation{}, err
	}
	return ProjectActivation(binding)
}

// OriginalActivation locates a retained entry without selecting collaborators.
// Transitive membership does not grant a new root entry into another Unit.
func OriginalActivation(activations []plugin.Activation, installation identity.InstallationID, release identity.Digest, server mcpserver.ServerName) (plugin.Activation, error) {
	source := qimu.UnitRef{InstallationID: qimu.InstallationID(installation.String()), Release: qimu.Digest(release.String()), UnitID: BackendUnit(server)}
	var selected *plugin.Activation
	for _, activation := range activations {
		binding, err := DecodeActivation(activation)
		if err != nil {
			return plugin.Activation{}, err
		}
		root := slices.Contains(binding.UnitRoots, source) || slices.ContainsFunc(binding.Roots, func(root qimu.ExportRef) bool { return root.Unit() == source })
		if !root {
			continue
		}
		if selected != nil && !selected.Equal(activation) {
			return plugin.Activation{}, fmt.Errorf("%w: multiple retained setup bindings for the same entry", plugin.ErrStale)
		}
		value := activation.Clone()
		selected = &value
	}
	if selected == nil {
		return plugin.Activation{}, fmt.Errorf("%w: original entry binding is missing", plugin.ErrStale)
	}
	return selected.Clone(), nil
}

func BindActivations(bindings []ContributionBinding, activations []plugin.Activation) ([]ContributionBinding, error) {
	result := slices.Clone(bindings)
	for index, binding := range result {
		activation, err := OriginalActivation(activations, binding.InstallationID, binding.Digest, binding.ServerName)
		if err != nil {
			return nil, err
		}
		result[index].Activation = &activation
	}
	return result, nil
}
