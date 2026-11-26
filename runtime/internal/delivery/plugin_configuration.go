package delivery

import (
	"cmp"
	"context"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framevalue "github.com/Tangerg/frame/value"
	skillspec "github.com/Tangerg/scope/skills"
)

type installationConfiguration struct {
	installation identity.InstallationID
	digest       framevalue.Digest
	changes      plugin.Configuration
}

func preparePluginConfiguration(in protocol.ConfigurePluginRequest) (installationConfiguration, error) {
	installation, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return installationConfiguration{}, err
	}
	changes := plugin.Configuration{
		Values:           make(map[string]plugin.ValueChange, len(in.ValueChanges)),
		ServerEnablement: make(map[mcpserver.ServerName]bool, len(in.ServerChanges)),
		SkillEnablement:  make(map[string]bool, len(in.SkillChanges)),
	}
	var violations []protocol.FieldError
	invalidName := func(field, key, detail string) {
		violations = append(violations, protocol.FieldError{Field: framevalue.MapPath(field, key), Detail: detail})
	}
	for id, change := range in.ValueChanges {
		if !identity.ValidDeclarationID(id) {
			invalidName("valueChanges", id, "must match "+identity.DeclarationIDPattern)
			continue
		}
		if change.Type == protocol.PluginValueSet {
			changes.Values[id] = plugin.SetValue(*change.Value)
		} else {
			changes.Values[id] = plugin.ClearValue()
		}
	}
	for raw, change := range in.ServerChanges {
		name, err := mcpserver.ParseServerName(raw)
		if err != nil {
			invalidName("serverChanges", raw, err.Error())
			continue
		}
		changes.ServerEnablement[name] = change == protocol.PluginComponentEnable
	}
	for name, change := range in.SkillChanges {
		if err := skillspec.ValidateName(name); err != nil {
			invalidName("skillChanges", name, err.Error())
			continue
		}
		changes.SkillEnablement[name] = change == protocol.PluginComponentEnable
	}
	if len(violations) != 0 {
		slices.SortFunc(violations, func(a, b protocol.FieldError) int { return cmp.Compare(a.Field, b.Field) })
		return installationConfiguration{}, protocol.InvalidParameters(&protocol.ConstraintError{Fields: violations})
	}
	return installationConfiguration{installation: installation, digest: digest, changes: changes}, nil
}

func (s *Handler) ConfigurePlugin(ctx context.Context, in protocol.ConfigurePluginRequest) (*protocol.PluginReleaseRef, error) {
	configuration, err := preparePluginConfiguration(in)
	if err != nil {
		return nil, err
	}
	var digest framevalue.Digest
	if err := s.configurePlugin(ctx, configuration, operation.DirectReply(&digest)); err != nil {
		return nil, err
	}
	return presentPluginReleaseRef(configuration.installation.String(), digest), nil
}

func (s *Handler) replayPluginConfiguration(ctx context.Context, in protocol.ConfigurePluginRequest, call keyedCall) protocol.OperationResult {
	configuration, err := preparePluginConfiguration(in)
	if err != nil {
		return failed(projectError(err))
	}
	return call.invokeAtomic(ctx, func(outcome atomicOutcome) error {
		return outcome.settleRefusal(ctx, s.configurePlugin(ctx, configuration, outcome.pluginRelease(configuration.installation.String())))
	})
}

func (s *Handler) configurePlugin(ctx context.Context, configuration installationConfiguration, reply operation.Reply[framevalue.Digest]) error {
	return wirePluginError(s.plugins.Configure(ctx, configuration.installation, configuration.digest, configuration.changes, reply))
}
