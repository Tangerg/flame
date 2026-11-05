package pluginbrowser

import (
	framebrowser "github.com/Tangerg/frame/browser"
	framevalue "github.com/Tangerg/frame/value"

	"context"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame"

	"github.com/Tangerg/frame/distribution"

	"github.com/samber/lo"
)

type resources interface {
	ReadResource(context.Context, plugin.Installation, string) ([]byte, error)
	ComponentModules(context.Context, plugin.Release, plugin.ContributionID) (*distribution.Module, error)
}

// Source projects an already approved component from Flame's immutable release.
// It has no admission, Realm, instance or resource registry of its own.
type Source struct {
	installations plugins.Store
	resources     resources
}

func NewSource(installations plugins.Store, resources resources) (*Source, error) {
	if lo.IsNil(installations) || lo.IsNil(resources) {
		return nil, errors.New("plugin browser: original installation and release resource owners are required")
	}
	return &Source{installations: installations, resources: resources}, nil
}

func (s *Source) component(ctx context.Context, binding frame.UnitBinding) (plugin.Snapshot, plugin.ClientComponent, error) {
	id, err := identity.ParseHostInstallation(binding.Source.InstallationID)
	if err != nil {
		return plugin.Snapshot{}, plugin.ClientComponent{}, err
	}
	componentID, err := plugins.ComponentID(binding.Source.UnitID)
	if err != nil {
		return plugin.Snapshot{}, plugin.ClientComponent{}, err
	}
	digest := binding.Source.Release
	if err := digest.Validate(); err != nil {
		return plugin.Snapshot{}, plugin.ClientComponent{}, err
	}
	snapshot, found, err := s.installations.Get(ctx, id)
	if err != nil {
		return plugin.Snapshot{}, plugin.ClientComponent{}, err
	}
	if !found {
		return plugin.Snapshot{}, plugin.ClientComponent{}, plugin.ErrNotFound
	}
	component, err := snapshot.Installation.AuthorizeComponent(snapshot.Selected, digest, componentID)
	if err != nil {
		return plugin.Snapshot{}, plugin.ClientComponent{}, err
	}
	configuration, err := plugins.EncodeFrameworkConfiguration(snapshot.Installation.Snapshot())
	if err != nil || framevalue.Hash(configuration) != binding.Configuration {
		return plugin.Snapshot{}, plugin.ClientComponent{}, errors.Join(frame.ErrStaleBinding, err)
	}
	return snapshot, component, nil
}

func (s *Source) Module(ctx context.Context, binding frame.UnitBinding) (framebrowser.Artifact, error) {
	snapshot, component, err := s.component(ctx, binding)
	if err != nil {
		return framebrowser.Artifact{}, err
	}
	content, err := s.resources.ReadResource(ctx, snapshot.Installation, component.Entry)
	if err != nil {
		return framebrowser.Artifact{}, err
	}
	return framebrowser.Artifact{Path: component.Entry, Digest: framevalue.Hash(content)}, nil
}

func (s *Source) Configuration(ctx context.Context, setup *frame.Setup) ([]byte, error) {
	if setup == nil {
		return nil, plugin.ErrInvalid
	}
	snapshot, component, err := s.component(ctx, setup.Binding())
	if err != nil {
		return nil, err
	}
	projection, err := plugins.PublicComponentConfiguration(snapshot.Selected, component.ID)
	if err != nil {
		return nil, err
	}
	var value plugins.ComponentConfiguration
	if err := json.Unmarshal(projection, &value); err != nil {
		return nil, err
	}
	module, err := s.resources.ComponentModules(ctx, snapshot.Selected, component.ID)
	if err != nil {
		return nil, err
	}
	if module == nil || module.Entry != component.Entry {
		return nil, frame.ErrContractMismatch
	}
	value.SharedModules = []string{}
	for _, imported := range module.Imports {
		if imported.Shared == nil {
			continue
		}
		if value.ModuleProfile != "" && value.ModuleProfile != imported.Shared.Profile {
			return nil, frame.ErrContractMismatch
		}
		value.ModuleProfile = imported.Shared.Profile
		value.SharedModules = append(value.SharedModules, imported.Specifier)
	}
	encoded, err := framevalue.EncodeJSON(value)
	if err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}
