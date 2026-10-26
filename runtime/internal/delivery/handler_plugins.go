package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type pluginUseCases interface {
	List(context.Context) ([]plugins.Inspection, error)
	Install(context.Context, string) (plugins.Inspection, error)
	Stage(context.Context, identity.InstallationID, string) (plugins.Inspection, error)
	Select(context.Context, identity.InstallationID, identity.Digest) (plugins.Inspection, error)
	Enable(context.Context, identity.InstallationID) (plugins.Inspection, error)
	Disable(context.Context, identity.InstallationID) (plugins.Inspection, error)
	Configure(context.Context, identity.InstallationID, identity.Digest, plugin.Configuration) (plugins.Inspection, error)
	Uninstall(context.Context, identity.InstallationID) error
	Component(context.Context, identity.InstallationID, identity.Digest, plugin.ContributionID) (plugin.ClientComponent, error)
	Resource(context.Context, identity.InstallationID, identity.Digest, plugin.ContributionID, string) ([]byte, error)
}

type selectionUseCases interface {
	List(context.Context) ([]plugin.ProviderSelection, error)
	Select(context.Context, plugin.SelectionRecord) (plugin.ProviderSelection, error)
}

func presentProviderSelection(value plugin.ProviderSelection) *protocol.PluginProviderSelection {
	record := value.Snapshot()
	return &protocol.PluginProviderSelection{Role: record.Role, InstallationID: record.Installation.String(), Contribution: record.Contribution.String()}
}

func (s *Handler) ListPluginProviders(ctx context.Context) (*protocol.Page[protocol.PluginProviderSelection], error) {
	if s.pluginSelections == nil {
		return nil, protocol.NewFailure(protocol.ErrPluginUnavailable, "plugin provider selection is unavailable")
	}
	selections, err := s.pluginSelections.List(ctx)
	if err != nil {
		return nil, wirePluginError(err)
	}
	result := make([]protocol.PluginProviderSelection, 0, len(selections))
	for _, selection := range selections {
		result = append(result, *presentProviderSelection(selection))
	}
	return protocol.NewPage(result), nil
}

func (s *Handler) SelectPluginProvider(ctx context.Context, in protocol.SelectPluginProviderRequest) (*protocol.PluginProviderSelection, error) {
	if s.pluginSelections == nil {
		return nil, protocol.NewFailure(protocol.ErrPluginUnavailable, "plugin provider selection is unavailable")
	}
	id, err := parseInstallationID(in.InstallationID)
	if err != nil {
		return nil, err
	}
	contribution, err := plugin.ParseContributionID(in.Contribution)
	if err != nil {
		return nil, protocol.InvalidParameters(err)
	}
	value, err := s.pluginSelections.Select(ctx, plugin.SelectionRecord{Role: in.Role, Installation: id, Contribution: contribution})
	if err != nil {
		return nil, wirePluginError(err)
	}
	return presentProviderSelection(value), nil
}

func parseInstallationID(text string) (identity.InstallationID, error) {
	id, err := identity.ParseInstallation(text)
	if err != nil {
		return identity.InstallationID{}, protocol.InvalidParameters(err)
	}
	return id, nil
}

func parseReleaseDigest(text string) (identity.Digest, error) {
	digest, err := identity.ParseDigest(text)
	if err != nil {
		return identity.Digest{}, protocol.InvalidParameters(err)
	}
	return digest, nil
}

func parseInstallationRelease(id, digest string) (identity.InstallationID, identity.Digest, error) {
	installation, err := parseInstallationID(id)
	if err != nil {
		return identity.InstallationID{}, identity.Digest{}, err
	}
	release, err := parseReleaseDigest(digest)
	return installation, release, err
}

func wirePluginError(err error) error {
	switch {
	case errors.Is(err, plugin.ErrCommandRetired):
		return protocol.NewFailure(errors.Join(protocol.ErrPluginCommandRetired, err), err.Error())
	case errors.Is(err, plugin.ErrCommandUnconfirmed):
		// A lost acknowledgement leaves the command's outcome unknown: it must not be
		// resubmitted or retargeted, so the category stays the non-retryable internal
		// failure. The generic command path projects through protocol.ProjectError, which has
		// no plugin spec and would otherwise mint a fresh internal error that drops
		// the cause; join it so the unconfirmed cause stays reachable.
		return protocol.NewFailure(errors.Join(protocol.ErrInternalError, err), err.Error())
	case errors.Is(err, plugin.ErrInvalid):
		return protocol.InvalidParameters(err)
	case errors.Is(err, plugin.ErrUnavailable):
		return protocol.NewFailure(errors.Join(protocol.ErrPluginUnavailable, err), err.Error())
	case errors.Is(err, plugin.ErrNotFound):
		return protocol.NewFailure(errors.Join(protocol.ErrPluginNotFound, err), err.Error())
	case errors.Is(err, plugin.ErrInUse):
		return protocol.NewFailure(errors.Join(protocol.ErrPluginInUse, err), err.Error())
	case errors.Is(err, plugin.ErrDisabled):
		return protocol.NewFailure(errors.Join(protocol.ErrPluginDisabled, err), err.Error())
	case errors.Is(err, plugin.ErrStale):
		return protocol.NewFailure(errors.Join(protocol.ErrPluginStale, err), err.Error())
	default:
		return err
	}
}
func presentPluginResult(inspection plugins.Inspection, err error) (*protocol.PluginInstallation, error) {
	if err != nil {
		return nil, wirePluginError(err)
	}
	return presentInstallation(inspection)
}
func (s *Handler) ListPlugins(ctx context.Context) (*protocol.Page[protocol.PluginInstallation], error) {
	records, err := s.plugins.List(ctx)
	if err != nil {
		return nil, wirePluginError(err)
	}
	result := make([]protocol.PluginInstallation, 0, len(records))
	for _, inspection := range records {
		value, err := presentInstallation(inspection)
		if err != nil {
			return nil, err
		}
		result = append(result, *value)
	}
	return protocol.NewPage(result), nil
}
func (s *Handler) InstallPlugin(ctx context.Context, in protocol.InstallPluginRequest) (*protocol.PluginInstallation, error) {
	return presentPluginResult(s.plugins.Install(ctx, in.Source))
}
func (s *Handler) StagePlugin(ctx context.Context, in protocol.StagePluginRequest) (*protocol.PluginInstallation, error) {
	id, err := parseInstallationID(in.InstallationID)
	if err != nil {
		return nil, err
	}
	return presentPluginResult(s.plugins.Stage(ctx, id, in.Source))
}
func (s *Handler) SelectPlugin(ctx context.Context, in protocol.PluginReleaseRequest) (*protocol.PluginInstallation, error) {
	id, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return nil, err
	}
	return presentPluginResult(s.plugins.Select(ctx, id, digest))
}
func (s *Handler) ConfigurePlugin(ctx context.Context, in protocol.ConfigurePluginRequest) (*protocol.PluginInstallation, error) {
	id, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return nil, err
	}
	configuration := plugin.Configuration{
		Values:  make(map[string]plugin.ValueChange, len(in.ValueChanges)),
		Servers: make(map[mcpserver.ServerName]plugin.ComponentChange, len(in.ServerChanges)),
		Skills:  make(map[string]plugin.ComponentChange, len(in.SkillChanges)),
	}
	for key, change := range in.ValueChanges {
		switch {
		case change.Type == protocol.PluginValueSet && change.Value != nil:
			configuration.Values[key] = plugin.SetValue(*change.Value)
		case change.Type == protocol.PluginValueClear && change.Value == nil:
			configuration.Values[key] = plugin.ClearValue()
		default:
			return nil, protocol.InvalidParameters(fmt.Errorf("plugin input %q change", key))
		}
	}
	for raw, change := range in.ServerChanges {
		name, err := mcpserver.ParseServerName(raw)
		if err != nil {
			return nil, protocol.InvalidParameters(err)
		}
		configuration.Servers[name], err = parseComponentChange(change)
		if err != nil {
			return nil, err
		}
	}
	for name, change := range in.SkillChanges {
		configuration.Skills[name], err = parseComponentChange(change)
		if err != nil {
			return nil, err
		}
	}
	return presentPluginResult(s.plugins.Configure(ctx, id, digest, configuration))
}

func parseComponentChange(change protocol.PluginComponentChange) (plugin.ComponentChange, error) {
	switch change {
	case protocol.PluginComponentEnable:
		return plugin.EnableComponent, nil
	case protocol.PluginComponentDisable:
		return plugin.DisableComponent, nil
	default:
		return "", protocol.InvalidParameters(fmt.Errorf("plugin component change %q", change))
	}
}
func (s *Handler) SetPluginEnablement(ctx context.Context, in protocol.SetPluginEnablementRequest) (*protocol.PluginInstallation, error) {
	id, err := parseInstallationID(in.InstallationID)
	if err != nil {
		return nil, err
	}
	if in.Enabled {
		return presentPluginResult(s.plugins.Enable(ctx, id))
	}
	return presentPluginResult(s.plugins.Disable(ctx, id))
}
func (s *Handler) UninstallPlugin(ctx context.Context, in protocol.PluginRequest) error {
	id, err := parseInstallationID(in.InstallationID)
	if err != nil {
		return err
	}
	return wirePluginError(s.plugins.Uninstall(ctx, id))
}
