package delivery

import (
	"context"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func (s *Handler) ResolvePluginComposition(ctx context.Context, in protocol.ResolvePluginCompositionRequest) (*protocol.Page[protocol.PluginOperationBinding], error) {
	contract, err := plugincontract.ParseCapability([]byte(in.Contract))
	if err != nil {
		return nil, protocol.InvalidParameters(err)
	}
	bindings, err := s.pluginCompositions.Resolve(ctx, contract)
	if err != nil {
		return nil, wirePluginError(err)
	}
	rows := make([]protocol.PluginOperationBinding, len(bindings))
	for index, binding := range bindings {
		rows[index] = protocol.PluginOperationBinding{InstallationID: binding.InstallationID.String(), Digest: binding.Digest.String(), Contribution: binding.ContributionID.String(), Contract: string(binding.Contract.Bytes())}
	}
	return protocol.NewPage(rows), nil
}

func presentComposition(value plugin.OrderedComposition) *protocol.PluginComposition {
	order := value.Members()
	members := make([]protocol.PluginCompositionMember, len(order))
	for index, member := range order {
		members[index] = protocol.PluginCompositionMember{InstallationID: member.Installation.String(), Contribution: member.Contribution.String()}
	}
	return &protocol.PluginComposition{Contract: value.ContractIdentity().String(), Members: members}
}

func (s *Handler) ListPluginCompositions(ctx context.Context) (*protocol.Page[protocol.PluginComposition], error) {
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
	var value plugin.OrderedComposition
	if err := s.setPluginComposition(ctx, in, operation.DirectReply(&value)); err != nil {
		return nil, err
	}
	return presentComposition(value), nil
}

func (s *Handler) commitPluginComposition(ctx context.Context, in protocol.SetPluginCompositionRequest, outcome atomicOutcome) error {
	reply := atomicReply(outcome.complete, func(value plugin.OrderedComposition) ([]byte, error) {
		return encodeAtomicResult(outcome.method, presentComposition(value))
	})
	return outcome.settleRefusal(ctx, s.setPluginComposition(ctx, in, reply))
}

func (s *Handler) setPluginComposition(ctx context.Context, in protocol.SetPluginCompositionRequest, reply operation.Reply[plugin.OrderedComposition]) error {
	members := make([]plugin.CompositionMember, len(in.Members))
	for index, member := range in.Members {
		installation, err := parseInstallationID(member.InstallationID)
		if err != nil {
			return err
		}
		contribution, err := plugin.ParseContributionID(member.Contribution)
		if err != nil {
			return protocol.InvalidParameters(err)
		}
		members[index] = plugin.CompositionMember{Installation: installation, Contribution: contribution}
	}
	value, err := plugin.NewOrderedComposition([]byte(in.Contract), members)
	if err != nil {
		return wirePluginError(err)
	}
	return wirePluginError(s.pluginCompositions.Set(ctx, value, reply))
}
