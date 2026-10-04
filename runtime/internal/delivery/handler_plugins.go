package delivery

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/protocol"
)

type pluginUseCases interface {
	List(context.Context) ([]plugins.Inspection, error)
	Install(context.Context, string) (plugins.Inspection, error)
	Stage(context.Context, string, string) (plugins.Inspection, error)
	Select(context.Context, string, string) (plugins.Inspection, error)
	Approve(context.Context, string, string, []plugin.RequestGrant) (plugins.Inspection, error)
	Enable(context.Context, string, bool) (plugins.Inspection, error)
	Revoke(context.Context, string) (plugins.Inspection, error)
	Configure(context.Context, string, plugin.Configuration) (plugins.Inspection, error)
	Uninstall(context.Context, string) (plugins.Removal, error)
}

func wirePluginError(err error) error {
	switch {
	case errors.Is(err, plugin.ErrInvalid):
		return InvalidParameters(err)
	case errors.Is(err, plugin.ErrUnavailable):
		return errors.Join(protocol.ErrPluginUnavailable, err)
	case errors.Is(err, plugin.ErrNotFound):
		return errors.Join(protocol.ErrPluginNotFound, err)
	case errors.Is(err, plugin.ErrInUse):
		return errors.Join(protocol.ErrPluginInUse, err)
	case errors.Is(err, plugin.ErrUnapproved):
		return errors.Join(protocol.ErrPluginUnapproved, err)
	case errors.Is(err, plugin.ErrStale):
		return errors.Join(protocol.ErrPluginStale, err)
	default:
		return err
	}
}
func presentPluginResult(inspection plugins.Inspection, err error) (*protocol.PluginInstallation, error) {
	if err != nil {
		return nil, wirePluginError(err)
	}
	value := presentInstallation(inspection.Record)
	for _, diagnostic := range inspection.Availability {
		value.Availability = append(value.Availability, presentPluginDiagnostic(diagnostic))
	}
	return value, nil
}
func (s *Handler) ListPlugins(ctx context.Context) (*protocol.Page[protocol.PluginInstallation], error) {
	records, err := s.plugins.List(ctx)
	if err != nil {
		return nil, wirePluginError(err)
	}
	result := make([]protocol.PluginInstallation, 0, len(records))
	for _, r := range records {
		value := presentInstallation(r.Record)
		for _, diagnostic := range r.Availability {
			value.Availability = append(value.Availability, presentPluginDiagnostic(diagnostic))
		}
		result = append(result, *value)
	}
	return protocol.NewPage(result), nil
}
func (s *Handler) InstallPlugin(ctx context.Context, in protocol.InstallPluginRequest) (*protocol.PluginInstallation, error) {
	return presentPluginResult(s.plugins.Install(ctx, in.Source))
}
func (s *Handler) StagePlugin(ctx context.Context, in protocol.StagePluginRequest) (*protocol.PluginInstallation, error) {
	return presentPluginResult(s.plugins.Stage(ctx, in.InstallationID, in.Source))
}
func (s *Handler) SelectPlugin(ctx context.Context, in protocol.PluginReleaseRequest) (*protocol.PluginInstallation, error) {
	return presentPluginResult(s.plugins.Select(ctx, in.InstallationID, in.Digest))
}
func (s *Handler) ApprovePlugin(ctx context.Context, in protocol.ApprovePluginRequest) (*protocol.PluginInstallation, error) {
	grants := make([]plugin.RequestGrant, 0, len(in.Grants))
	for _, g := range in.Grants {
		grants = append(grants, plugin.RequestGrant{Capability: plugin.Capability(g.Capability), Targets: g.Targets})
	}
	return presentPluginResult(s.plugins.Approve(ctx, in.InstallationID, in.Digest, grants))
}
func (s *Handler) ConfigurePlugin(ctx context.Context, in protocol.ConfigurePluginRequest) (*protocol.PluginInstallation, error) {
	changes := make(map[string]*string, len(in.ValueChanges))
	for key, value := range in.ValueChanges {
		switch value.Type {
		case protocol.PluginValueSet:
			changes[key] = value.Value
		case protocol.PluginValueClear:
			changes[key] = nil
		default:
			return nil, protocol.ErrInvalidParams
		}
	}
	return presentPluginResult(s.plugins.Configure(ctx, in.InstallationID, plugin.Configuration{
		Digest: in.Digest, Values: changes, DisabledServers: in.DisabledServers, DisabledSkills: in.DisabledSkills,
	}))
}
func (s *Handler) SetPluginEnablement(ctx context.Context, in protocol.SetPluginEnablementRequest) (*protocol.PluginInstallation, error) {
	return presentPluginResult(s.plugins.Enable(ctx, in.InstallationID, in.Enabled))
}
func (s *Handler) RevokePlugin(ctx context.Context, in protocol.PluginRequest) (*protocol.PluginInstallation, error) {
	return presentPluginResult(s.plugins.Revoke(ctx, in.InstallationID))
}
func (s *Handler) UninstallPlugin(ctx context.Context, in protocol.PluginRequest) (*protocol.PluginRemoval, error) {
	removed, err := s.plugins.Uninstall(ctx, in.InstallationID)
	if err != nil {
		return nil, wirePluginError(err)
	}
	result := &protocol.PluginRemoval{Availability: []protocol.PluginDiagnostic{}}
	for _, diagnostic := range removed.Availability {
		result.Availability = append(result.Availability, presentPluginDiagnostic(diagnostic))
	}
	return result, nil
}
