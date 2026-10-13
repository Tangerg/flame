package delivery

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/flame/runtime/point"
	"github.com/Tangerg/flame/runtime/protocol"
)

type pluginCallUseCases interface {
	Prepare(context.Context, plugins.CallRequest) (plugins.Call, error)
	Resolve(context.Context, resourceid.InstallationID, fingerprint.Digest, plugin.ContributionID) ([]plugins.Collaborator, error)
	ResolvePoint(context.Context, *point.Contract, point.Context, []plugins.PointReference) ([]plugins.PointBinding, error)
}

func (s *Handler) ResolvePluginPoint(ctx context.Context, in protocol.ResolvePluginPointRequest) (*protocol.Page[protocol.PluginPointBinding], error) {
	if s.pluginCalls == nil {
		return nil, NewFailure(protocol.ErrPluginUnavailable, "plugin points are unavailable")
	}
	contract, err := point.Admit(in.Contract)
	if err != nil {
		return nil, InvalidParameters(err)
	}
	order := make([]plugins.PointReference, 0, len(in.Order))
	for _, value := range in.Order {
		installation, err := resourceid.ParseInstallation(value.InstallationID)
		if err != nil {
			return nil, InvalidParameters(err)
		}
		contribution, err := plugin.ParseContributionID(value.Contribution)
		if err != nil {
			return nil, InvalidParameters(err)
		}
		order = append(order, plugins.PointReference{Installation: installation, Contribution: contribution})
	}
	values, err := s.pluginCalls.ResolvePoint(ctx, contract, point.Context{Target: in.Target, Attributes: in.Attributes}, order)
	if err != nil {
		return nil, wirePluginError(err)
	}
	result := make([]protocol.PluginPointBinding, 0, len(values))
	for _, value := range values {
		result = append(result, protocol.PluginPointBinding{InstallationID: value.Reference.Installation.String(), Digest: value.Digest.String(), Contribution: value.Reference.Contribution.String(), Value: value.Value.JSON()})
	}
	return protocol.NewPage(result), nil
}

func (s *Handler) ResolvePluginRequirements(ctx context.Context, in protocol.ResolvePluginRequirementsRequest) (*protocol.Page[protocol.PluginCollaborator], error) {
	if s.pluginCalls == nil {
		return nil, NewFailure(protocol.ErrPluginUnavailable, "plugin requirements are unavailable")
	}
	id, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return nil, err
	}
	contribution, err := plugin.ParseContributionID(in.Contribution)
	if err != nil {
		return nil, InvalidParameters(err)
	}
	resolved, err := s.pluginCalls.Resolve(ctx, id, digest, contribution)
	if err != nil {
		return nil, wirePluginError(err)
	}
	values := make([]protocol.PluginCollaborator, 0, len(resolved))
	for _, collaborator := range resolved {
		value := protocol.PluginCollaborator{ID: collaborator.ID.String(), Bindings: []protocol.PluginOperationBinding{}}
		for _, binding := range collaborator.Bindings {
			contract, err := capability.Parse(binding.Contract.JSON())
			if err != nil {
				return nil, err
			}
			value.Bindings = append(value.Bindings, protocol.PluginOperationBinding{InstallationID: binding.InstallationID.String(), Digest: binding.Digest.String(), Contribution: binding.ContributionID.String(), Contract: contract.Descriptor()})
		}
		values = append(values, value)
	}
	return protocol.NewPage(values), nil
}

func pluginCallOperation(in protocol.CallPluginRequest) (OperationKind, error) {
	contract, err := capability.FromDescriptor(in.Contract)
	if err != nil {
		return "", InvalidParameters(err)
	}
	method, found := contract.Method(in.Method)
	if !found {
		return "", InvalidParameters(plugin.ErrInvalid)
	}
	if method.Kind() == capability.Mutation {
		return OperationCommand, nil
	}
	return OperationQuery, nil
}

func (s *Handler) PreparePluginCall(ctx context.Context, in protocol.CallPluginRequest) (OperationKind, func(context.Context) (*protocol.CallPluginResponse, error), error) {
	if s.pluginCalls == nil {
		return "", nil, NewFailure(protocol.ErrPluginUnavailable, "plugin calls are unavailable")
	}
	id, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return "", nil, err
	}
	contribution, err := plugin.ParseContributionID(in.Contribution)
	if err != nil {
		return "", nil, InvalidParameters(err)
	}
	contract, err := capability.FromDescriptor(in.Contract)
	if err != nil {
		return "", nil, InvalidParameters(err)
	}
	input, err := jsoninput.Parse(in.Input)
	if err != nil {
		return "", nil, InvalidParameters(err)
	}
	call, err := s.pluginCalls.Prepare(ctx, plugins.CallRequest{Installation: id, Digest: digest, Contribution: contribution, Contract: contract, Method: in.Method, Input: input, Target: in.Target})
	if err != nil {
		return "", nil, wirePluginError(err)
	}
	kind := OperationQuery
	if call.Kind() == capability.Mutation {
		kind = OperationCommand
	}
	return kind, func(ctx context.Context) (*protocol.CallPluginResponse, error) {
		output, err := call.Invoke(ctx)
		if failure, known := errors.AsType[*pluginsdk.Failure](err); known && !errors.Is(err, plugin.ErrCommandUnconfirmed) {
			return &protocol.CallPluginResponse{Failure: &protocol.PluginCallFailure{Code: failure.Code, Message: failure.Message}}, nil
		}
		if err != nil {
			return nil, wirePluginError(err)
		}
		return &protocol.CallPluginResponse{Output: output.JSON()}, nil
	}, nil
}
