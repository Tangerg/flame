package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/protocol"
)

type compositionUseCases interface {
	List(context.Context) ([]plugin.OrderedComposition, error)
	Set(context.Context, plugin.CompositionRecord) (plugin.OrderedComposition, error)
	Resolve(context.Context, *capability.Contract) ([]plugins.ContributionBinding, error)
}

func (s *Handler) ResolvePluginComposition(ctx context.Context, in protocol.ResolvePluginCompositionRequest) (*protocol.Page[protocol.PluginOperationBinding], error) {
	if s.pluginCompositions == nil {
		return nil, NewFailure(protocol.ErrPluginUnavailable, "plugin composition is unavailable")
	}
	contract, err := capability.Parse([]byte(in.Contract))
	if err != nil {
		return nil, InvalidParameters(err)
	}
	bindings, err := s.pluginCompositions.Resolve(ctx, contract)
	if err != nil {
		return nil, wirePluginError(err)
	}
	rows := make([]protocol.PluginOperationBinding, len(bindings))
	for index, binding := range bindings {
		rows[index] = protocol.PluginOperationBinding{InstallationID: binding.InstallationID.String(), Digest: binding.Digest.String(), Contribution: binding.ContributionID.String(), Contract: string(binding.Contract.JSON())}
	}
	return protocol.NewPage(rows), nil
}

func presentComposition(value plugin.OrderedComposition) *protocol.PluginComposition {
	record := value.Snapshot()
	members := make([]protocol.PluginCompositionMember, len(record.Members))
	for index, member := range record.Members {
		members[index] = protocol.PluginCompositionMember{InstallationID: member.Installation.String(), Contribution: member.Contribution.String()}
	}
	return &protocol.PluginComposition{Contract: record.Contract.String(), Members: members}
}

func (s *Handler) ListPluginCompositions(ctx context.Context) (*protocol.Page[protocol.PluginComposition], error) {
	if s.pluginCompositions == nil {
		return nil, NewFailure(protocol.ErrPluginUnavailable, "plugin composition is unavailable")
	}
	values, err := s.pluginCompositions.List(ctx)
	if err != nil {
		return nil, wirePluginError(err)
	}
	rows := make([]protocol.PluginComposition, len(values))
	for index, value := range values {
		rows[index] = *presentComposition(value)
	}
	return protocol.NewPage(rows), nil
}

func (s *Handler) SetPluginComposition(ctx context.Context, in protocol.SetPluginCompositionRequest) (*protocol.PluginComposition, error) {
	if s.pluginCompositions == nil {
		return nil, NewFailure(protocol.ErrPluginUnavailable, "plugin composition is unavailable")
	}
	contract, err := jsoninput.Parse([]byte(in.Contract))
	if err != nil {
		return nil, InvalidParameters(err)
	}
	members := make([]plugin.CompositionMember, len(in.Members))
	for index, member := range in.Members {
		installation, err := parseInstallationID(member.InstallationID)
		if err != nil {
			return nil, err
		}
		contribution, err := plugin.ParseContributionID(member.Contribution)
		if err != nil {
			return nil, InvalidParameters(err)
		}
		members[index] = plugin.CompositionMember{Installation: installation, Contribution: contribution}
	}
	value, err := s.pluginCompositions.Set(ctx, plugin.CompositionRecord{Contract: contract, Members: members})
	if err != nil {
		return nil, wirePluginError(err)
	}
	return presentComposition(value), nil
}
