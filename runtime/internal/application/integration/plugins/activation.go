package plugins

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
)

func ProjectActivation(binding framehost.BindingSnapshot) (plugin.Activation, error) {
	canonical, err := binding.Canonical()
	if err != nil {
		return plugin.Activation{}, err
	}
	activation := plugin.Activation{Binding: canonical}
	for _, dependency := range binding.Dependencies() {
		installation, err := identity.ParseHostInstallation(dependency.InstallationID)
		if err != nil {
			return plugin.Activation{}, err
		}
		release := dependency.Release
		if err := release.Validate(); err != nil {
			return plugin.Activation{}, err
		}
		activation.Dependencies = append(activation.Dependencies, plugin.Dependency{InstallationID: installation, Digest: release})
	}
	return activation, activation.Validate()
}

func DecodeActivation(activation plugin.Activation) (framehost.BindingSnapshot, error) {
	if err := activation.Validate(); err != nil {
		return framehost.BindingSnapshot{}, err
	}
	binding, err := framehost.ParseBinding(activation.Binding.Bytes())
	if err != nil {
		return framehost.BindingSnapshot{}, err
	}
	original, err := ProjectActivation(binding)
	if err != nil {
		return framehost.BindingSnapshot{}, err
	}
	if original.Binding != activation.Binding || !slices.Equal(original.Dependencies, activation.Dependencies) {
		return framehost.BindingSnapshot{}, fmt.Errorf("%w: activation projection differs from its original binding", plugin.ErrInvalid)
	}
	return binding, nil
}

func ReadActivation(data []byte) (plugin.Activation, error) {
	binding, err := framehost.ParseBinding(data)
	if err != nil {
		return plugin.Activation{}, err
	}
	return ProjectActivation(binding)
}

func OriginalActivation(activations []plugin.Activation, installation identity.InstallationID, release framevalue.Digest, server mcpserver.ServerName) (plugin.Activation, error) {
	source := framehost.UnitRef{InstallationID: installation.Host(), Release: release, UnitID: BackendUnit(server)}
	var selected *plugin.Activation
	for _, activation := range activations {
		binding, err := DecodeActivation(activation)
		if err != nil {
			return plugin.Activation{}, err
		}
		root := slices.Contains(binding.UnitRoots, source) || slices.ContainsFunc(binding.Roots, func(root framehost.ExportRef) bool { return root.UnitRef == source })
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
