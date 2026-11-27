package pluginbrowser

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framebrowser "github.com/Tangerg/frame/browser"
	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/samber/lo"
)

type resources interface {
	ReadResource(context.Context, plugin.Installation, string) ([]byte, error)
	ComponentModuleImports(context.Context, plugin.Release, plugin.ClientComponent) (string, []string, error)
}

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

func (s *Source) component(ctx context.Context, binding framehost.UnitBinding) (plugin.Snapshot, plugin.ClientComponent, error) {
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
		return plugin.Snapshot{}, plugin.ClientComponent{}, errors.Join(framehost.ErrStaleBinding, err)
	}
	return snapshot, component, nil
}

func (s *Source) Module(ctx context.Context, binding framehost.UnitBinding) (framebrowser.Artifact, error) {
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

type componentConfiguration struct {
	ComponentID   string   `json:"componentId"`
	Export        string   `json:"export"`
	ModuleProfile string   `json:"moduleProfile"`
	SharedModules []string `json:"sharedModules"`
}

func (s *Source) Configuration(ctx context.Context, setup framehost.Setup) ([]byte, error) {
	if !setup.Valid() {
		return nil, plugin.ErrInvalid
	}
	snapshot, component, err := s.component(ctx, setup.Binding())
	if err != nil {
		return nil, err
	}
	profile, shared, err := s.resources.ComponentModuleImports(ctx, snapshot.Selected, component)
	if err != nil {
		return nil, err
	}
	encoded, err := framevalue.EncodeJSON(componentConfiguration{
		ComponentID: component.ID.String(), Export: component.Export,
		ModuleProfile: profile, SharedModules: shared,
	})
	if err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}
