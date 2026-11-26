package delivery

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framevalue "github.com/Tangerg/frame/value"
)

func presentProviderSelection(value plugin.ProviderSelection) *protocol.PluginProviderSelection {
	record := value.Snapshot()
	return &protocol.PluginProviderSelection{Role: record.Role, InstallationID: record.Installation.String(), Contribution: record.Contribution.String()}
}

func (s *Handler) ListPluginProviders(ctx context.Context) (*protocol.Page[protocol.PluginProviderSelection], error) {
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
	var selection plugin.ProviderSelection
	if err := s.selectPluginProvider(ctx, in, operation.DirectReply(&selection)); err != nil {
		return nil, err
	}
	return presentProviderSelection(selection), nil
}

func (s *Handler) commitPluginProviderSelection(ctx context.Context, in protocol.SelectPluginProviderRequest, outcome atomicOutcome) error {
	complete := atomicReply(outcome.complete, func(selection plugin.ProviderSelection) ([]byte, error) {
		return encodeAtomicResult(outcome.method, presentProviderSelection(selection))
	})
	return outcome.settleRefusal(ctx, s.selectPluginProvider(ctx, in, complete))
}

func (s *Handler) selectPluginProvider(ctx context.Context, in protocol.SelectPluginProviderRequest, complete operation.Reply[plugin.ProviderSelection]) error {
	id, err := parseInstallationID(in.InstallationID)
	if err != nil {
		return err
	}
	contribution, err := plugin.ParseContributionID(in.Contribution)
	if err != nil {
		return protocol.InvalidParameters(err)
	}
	return wirePluginError(s.pluginSelections.Select(ctx, plugin.SelectionRecord{Role: in.Role, Installation: id, Contribution: contribution}, complete))
}

func parseInstallationID(text string) (identity.InstallationID, error) {
	id, err := identity.ParseInstallation(text)
	if err != nil {
		return identity.InstallationID{}, protocol.InvalidParameters(err)
	}
	return id, nil
}

func parseReleaseDigest(text string) (framevalue.Digest, error) {
	digest, err := framevalue.ParseDigest(text)
	if err != nil {
		return framevalue.Digest{}, protocol.InvalidParameters(err)
	}
	return digest, nil
}

func parseInstallationRelease(id, digest string) (identity.InstallationID, framevalue.Digest, error) {
	installation, err := parseInstallationID(id)
	if err != nil {
		return identity.InstallationID{}, framevalue.Digest{}, err
	}
	release, err := parseReleaseDigest(digest)
	return installation, release, err
}

func wirePluginError(err error) error {
	switch {
	case errors.Is(err, plugin.ErrCommandRetired):
		return protocol.NewFailure(protocol.ErrPluginCommandRetired, err.Error(), err)
	case errors.Is(err, plugin.ErrCommandUnconfirmed):
		// protocol.ProjectError has no plugin spec and would mint a fresh internal error
		// that drops the cause, so the unconfirmed cause is joined.
		return protocol.NewFailure(protocol.ErrInternalError, err.Error(), err)
	case errors.Is(err, plugin.ErrInvalid):
		return protocol.InvalidParameters(err)
	case errors.Is(err, plugin.ErrUnavailable):
		return protocol.NewFailure(protocol.ErrPluginUnavailable, err.Error(), err)
	case errors.Is(err, plugin.ErrNotFound):
		return protocol.NewFailure(protocol.ErrPluginNotFound, err.Error(), err)
	case errors.Is(err, plugin.ErrInUse):
		return protocol.NewFailure(protocol.ErrPluginInUse, err.Error(), err)
	case errors.Is(err, plugin.ErrDisabled):
		return protocol.NewFailure(protocol.ErrPluginDisabled, err.Error(), err)
	case errors.Is(err, plugin.ErrStale):
		return protocol.NewFailure(protocol.ErrPluginStale, err.Error(), err)
	default:
		return err
	}
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

func (s *Handler) InstallPlugin(ctx context.Context, in protocol.InstallPluginRequest) (*protocol.PluginReleaseRef, error) {
	var installation plugin.Installation
	if err := s.plugins.Install(ctx, in.Source, operation.DirectReply(&installation)); err != nil {
		return nil, wirePluginError(err)
	}
	return presentPluginReleaseRef(installation.ID().String(), installation.Selected()), nil
}

func (s *Handler) commitPluginInstall(ctx context.Context, in protocol.InstallPluginRequest, outcome atomicOutcome) error {
	reply := atomicReply(outcome.complete, func(installation plugin.Installation) ([]byte, error) {
		return encodeAtomicResult(outcome.method, presentPluginReleaseRef(installation.ID().String(), installation.Selected()))
	})
	return outcome.settleRefusal(ctx, wirePluginError(s.plugins.Install(ctx, in.Source, reply)))
}

func (s *Handler) StagePlugin(ctx context.Context, in protocol.StagePluginRequest) (*protocol.PluginReleaseRef, error) {
	var staged plugin.StagedRelease
	if err := s.stagePlugin(ctx, in, operation.DirectReply(&staged)); err != nil {
		return nil, err
	}
	return presentStagedRelease(staged), nil
}

func (s *Handler) commitPluginStage(ctx context.Context, in protocol.StagePluginRequest, outcome atomicOutcome) error {
	reply := atomicReply(outcome.complete, func(staged plugin.StagedRelease) ([]byte, error) {
		return encodeAtomicResult(outcome.method, presentStagedRelease(staged))
	})
	return outcome.settleRefusal(ctx, s.stagePlugin(ctx, in, reply))
}

func (s *Handler) stagePlugin(ctx context.Context, in protocol.StagePluginRequest, reply operation.Reply[plugin.StagedRelease]) error {
	id, err := parseInstallationID(in.InstallationID)
	if err != nil {
		return err
	}
	return wirePluginError(s.plugins.Stage(ctx, id, in.Source, reply))
}

func presentStagedRelease(staged plugin.StagedRelease) *protocol.PluginReleaseRef {
	return presentPluginReleaseRef(staged.InstallationID().String(), staged.Digest())
}

func presentPluginReleaseRef(installationID string, digest framevalue.Digest) *protocol.PluginReleaseRef {
	return &protocol.PluginReleaseRef{InstallationID: installationID, Digest: digest.String()}
}

func (o atomicOutcome) pluginRelease(installationID string) operation.Reply[framevalue.Digest] {
	return atomicReply(o.complete, func(digest framevalue.Digest) ([]byte, error) {
		return encodeAtomicResult(o.method, presentPluginReleaseRef(installationID, digest))
	})
}

func (s *Handler) SelectPlugin(ctx context.Context, in protocol.PluginReleaseRef) (*protocol.PluginReleaseRef, error) {
	var digest framevalue.Digest
	if err := s.selectPlugin(ctx, in, operation.DirectReply(&digest)); err != nil {
		return nil, err
	}
	return presentPluginReleaseRef(in.InstallationID, digest), nil
}

func (s *Handler) commitPluginSelection(ctx context.Context, in protocol.PluginReleaseRef, outcome atomicOutcome) error {
	return outcome.settleRefusal(ctx, s.selectPlugin(ctx, in, outcome.pluginRelease(in.InstallationID)))
}

func (s *Handler) selectPlugin(ctx context.Context, in protocol.PluginReleaseRef, reply operation.Reply[framevalue.Digest]) error {
	id, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return err
	}
	return wirePluginError(s.plugins.Select(ctx, id, digest, reply))
}

func (s *Handler) ConfigurePlugin(ctx context.Context, in protocol.ConfigurePluginRequest) (*protocol.PluginReleaseRef, error) {
	var digest framevalue.Digest
	if err := s.configurePlugin(ctx, in, operation.DirectReply(&digest)); err != nil {
		return nil, err
	}
	return presentPluginReleaseRef(in.InstallationID, digest), nil
}

func (s *Handler) commitPluginConfiguration(ctx context.Context, in protocol.ConfigurePluginRequest, outcome atomicOutcome) error {
	return outcome.settleRefusal(ctx, s.configurePlugin(ctx, in, outcome.pluginRelease(in.InstallationID)))
}

func (s *Handler) configurePlugin(ctx context.Context, in protocol.ConfigurePluginRequest, reply operation.Reply[framevalue.Digest]) error {
	id, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return err
	}
	configuration := plugin.Configuration{
		Values:           make(map[string]plugin.ValueChange, len(in.ValueChanges)),
		ServerEnablement: make(map[mcpserver.ServerName]bool, len(in.ServerChanges)),
		SkillEnablement:  make(map[string]bool, len(in.SkillChanges)),
	}
	for key, change := range in.ValueChanges {
		if change.Type == protocol.PluginValueSet {
			configuration.Values[key] = plugin.SetValue(*change.Value)
		} else {
			configuration.Values[key] = plugin.ClearValue()
		}
	}
	for raw, change := range in.ServerChanges {
		name, err := mcpserver.ParseServerName(raw)
		if err != nil {
			return protocol.InvalidParameters(err)
		}
		configuration.ServerEnablement[name] = change == protocol.PluginComponentEnable
	}
	for name, change := range in.SkillChanges {
		configuration.SkillEnablement[name] = change == protocol.PluginComponentEnable
	}
	return wirePluginError(s.plugins.Configure(ctx, id, digest, configuration, reply))
}

func (s *Handler) SetPluginEnablement(ctx context.Context, in protocol.SetPluginEnablementRequest) (*protocol.PluginEnablement, error) {
	var enabled bool
	if err := s.setPluginEnablement(ctx, in, operation.DirectReply(&enabled)); err != nil {
		return nil, err
	}
	return presentPluginEnablement(in.InstallationID, enabled), nil
}

func (s *Handler) commitPluginEnablement(ctx context.Context, in protocol.SetPluginEnablementRequest, outcome atomicOutcome) error {
	reply := atomicReply(outcome.complete, func(enabled bool) ([]byte, error) {
		return encodeAtomicResult(outcome.method, presentPluginEnablement(in.InstallationID, enabled))
	})
	return outcome.settleRefusal(ctx, s.setPluginEnablement(ctx, in, reply))
}

func (s *Handler) setPluginEnablement(ctx context.Context, in protocol.SetPluginEnablementRequest, reply operation.Reply[bool]) error {
	id, err := parseInstallationID(in.InstallationID)
	if err != nil {
		return err
	}
	return wirePluginError(s.plugins.SetEnablement(ctx, id, in.Enabled, reply))
}

func presentPluginEnablement(installationID string, enabled bool) *protocol.PluginEnablement {
	return &protocol.PluginEnablement{InstallationID: installationID, Enabled: enabled}
}

func (s *Handler) UninstallPlugin(ctx context.Context, in protocol.PluginRequest) error {
	return s.uninstallPlugin(ctx, in, operation.DirectReply(new(struct{})))
}

func (s *Handler) commitPluginUninstall(ctx context.Context, in protocol.PluginRequest, outcome atomicOutcome) error {
	return outcome.settleRefusal(ctx, s.uninstallPlugin(ctx, in, outcome.acknowledgement()))
}

func (s *Handler) uninstallPlugin(ctx context.Context, in protocol.PluginRequest, reply operation.Reply[struct{}]) error {
	id, err := parseInstallationID(in.InstallationID)
	if err != nil {
		return err
	}
	return wirePluginError(s.plugins.Uninstall(ctx, id, reply))
}
