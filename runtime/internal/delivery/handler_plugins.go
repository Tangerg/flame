package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/protocol"
)

type pluginUseCases interface {
	List(context.Context) ([]plugins.Inspection, error)
	Install(context.Context, string) (plugins.Inspection, error)
	Stage(context.Context, resourceid.InstallationID, string) (plugins.Inspection, error)
	Select(context.Context, resourceid.InstallationID, fingerprint.Digest) (plugins.Inspection, error)
	Enable(context.Context, resourceid.InstallationID) (plugins.Inspection, error)
	Disable(context.Context, resourceid.InstallationID) (plugins.Inspection, error)
	Configure(context.Context, resourceid.InstallationID, fingerprint.Digest, plugin.Configuration) (plugins.Inspection, error)
	Uninstall(context.Context, resourceid.InstallationID) error
	ReadView(context.Context, resourceid.InstallationID, fingerprint.Digest, string) (string, error)
	ReadRenderer(context.Context, resourceid.InstallationID, fingerprint.Digest, string) (string, error)
	WithView(context.Context, resourceid.InstallationID, fingerprint.Digest, string, plugin.ViewKind, func(*plugin.Installation, plugin.ViewDeclaration) error) error
	AuthorizeAction(context.Context, resourceid.InstallationID, fingerprint.Digest, string, plugin.ActionOperation) error
	Component(context.Context, resourceid.InstallationID, fingerprint.Digest, plugin.ContributionID) (plugin.ClientComponent, error)
	Resource(context.Context, resourceid.InstallationID, fingerprint.Digest, plugin.ContributionID, string) ([]byte, error)
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
		return nil, NewFailure(protocol.ErrPluginUnavailable, "plugin provider selection is unavailable")
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
		return nil, NewFailure(protocol.ErrPluginUnavailable, "plugin provider selection is unavailable")
	}
	id, err := parseInstallationID(in.InstallationID)
	if err != nil {
		return nil, err
	}
	contribution, err := plugin.ParseContributionID(in.Contribution)
	if err != nil {
		return nil, InvalidParameters(err)
	}
	value, err := s.pluginSelections.Select(ctx, plugin.SelectionRecord{Role: in.Role, Installation: id, Contribution: contribution})
	if err != nil {
		return nil, wirePluginError(err)
	}
	return presentProviderSelection(value), nil
}

func (s *Handler) RenamePluginSession(ctx context.Context, in protocol.RenamePluginSessionRequest) (*protocol.Session, error) {
	id, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return nil, err
	}
	if err := s.plugins.AuthorizeAction(ctx, id, digest, in.ActionID, plugin.RenameSession); err != nil {
		return nil, wirePluginError(err)
	}
	return s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: in.Update.SessionID, ExpectedRevision: in.Update.ExpectedRevision, Title: &in.Update.Title})
}

func parseInstallationID(text string) (resourceid.InstallationID, error) {
	id, err := resourceid.ParseInstallation(text)
	if err != nil {
		return resourceid.InstallationID{}, InvalidParameters(err)
	}
	return id, nil
}

func parseReleaseDigest(text string) (fingerprint.Digest, error) {
	digest, err := fingerprint.ParseDigest(text)
	if err != nil {
		return fingerprint.Digest{}, InvalidParameters(err)
	}
	return digest, nil
}

func parseInstallationRelease(id, digest string) (resourceid.InstallationID, fingerprint.Digest, error) {
	installation, err := parseInstallationID(id)
	if err != nil {
		return resourceid.InstallationID{}, fingerprint.Digest{}, err
	}
	release, err := parseReleaseDigest(digest)
	return installation, release, err
}

func wirePluginError(err error) error {
	switch {
	case errors.Is(err, plugin.ErrCommandUnconfirmed):
		return err
	case errors.Is(err, plugin.ErrInvalid):
		return InvalidParameters(err)
	case errors.Is(err, plugin.ErrUnavailable):
		return NewFailure(errors.Join(protocol.ErrPluginUnavailable, err), err.Error())
	case errors.Is(err, plugin.ErrNotFound):
		return NewFailure(errors.Join(protocol.ErrPluginNotFound, err), err.Error())
	case errors.Is(err, plugin.ErrInUse):
		return NewFailure(errors.Join(protocol.ErrPluginInUse, err), err.Error())
	case errors.Is(err, plugin.ErrDisabled):
		return NewFailure(errors.Join(protocol.ErrPluginDisabled, err), err.Error())
	case errors.Is(err, plugin.ErrStale):
		return NewFailure(errors.Join(protocol.ErrPluginStale, err), err.Error())
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
			return nil, InvalidParameters(fmt.Errorf("plugin input %q change", key))
		}
	}
	for raw, change := range in.ServerChanges {
		name, err := mcpserver.ParseServerName(raw)
		if err != nil {
			return nil, InvalidParameters(err)
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
		return "", InvalidParameters(fmt.Errorf("plugin component change %q", change))
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

func (s *Handler) ReadPluginView(ctx context.Context, in protocol.ReadPluginViewRequest) (*protocol.PluginViewResource, error) {
	id, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return nil, err
	}
	html, err := s.plugins.ReadView(ctx, id, digest, in.ViewID)
	if err != nil {
		return nil, wirePluginError(err)
	}
	return &protocol.PluginViewResource{HTML: html}, nil
}

func (s *Handler) ReadPluginRenderer(ctx context.Context, in protocol.ReadPluginRendererRequest) (*protocol.PluginViewResource, error) {
	id, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return nil, err
	}
	html, err := s.plugins.ReadRenderer(ctx, id, digest, in.RendererID)
	if err != nil {
		return nil, wirePluginError(err)
	}
	return &protocol.PluginViewResource{HTML: html}, nil
}

func (s *Handler) ReadPluginSchedules(ctx context.Context, in protocol.ReadPluginSchedulesRequest) (page *protocol.Page[protocol.Schedule], err error) {
	id, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return nil, err
	}
	err = s.plugins.WithView(ctx, id, digest, in.ViewID, plugin.Schedules, func(*plugin.Installation, plugin.ViewDeclaration) error {
		var readErr error
		page, readErr = s.ListSchedules(ctx, in.PageQuery)
		return readErr
	})
	if err != nil {
		return nil, wirePluginError(err)
	}
	return page, nil
}
