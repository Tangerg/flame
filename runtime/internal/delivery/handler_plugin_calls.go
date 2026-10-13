package delivery

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/flame/runtime/protocol"
)

type pluginCallUseCases interface {
	Prepare(context.Context, plugins.CallRequest) (plugins.Call, error)
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
