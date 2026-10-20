package delivery

import (
	"context"
	"errors"
	"iter"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/point"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/flame/runtime/resourceid"
)

type pluginCallUseCases interface {
	Command(context.Context, string) (*plugin.Command, error)
	RetireCommand(context.Context, string) error
	NotifyChange(context.Context, plugin.ContributionID) error
	Prepare(context.Context, plugins.CallRequest) (plugins.Call, error)
	PrepareStream(context.Context, plugins.CallRequest) (plugins.StreamCall, error)
	Resolve(context.Context, resourceid.InstallationID, fingerprint.Digest, plugin.ContributionID) ([]plugins.Collaborator, error)
	ResolvePoint(context.Context, *point.Contract, point.Context, []plugins.PointReference) ([]plugins.PointBinding, error)
}

func (s *Handler) GetPluginCommand(ctx context.Context, in protocol.PluginCommandRequest) (*protocol.CallPluginRequest, error) {
	if in.Namespace != s.idempotency.Namespace {
		return nil, protocol.ErrIdempotencyStoreMismatch
	}
	if s.pluginCalls == nil {
		return nil, protocol.ErrPluginUnavailable
	}
	identity := operation.Identity(operation.Bind(ctx, s.idempotency.Namespace, operation.ScopedKey(ctx, in.Key)))
	command, err := s.pluginCalls.Command(ctx, identity)
	if err != nil {
		return nil, wirePluginError(err)
	}
	record := command.Snapshot()
	return &protocol.CallPluginRequest{
		InstallationID: record.Provider.InstallationID.String(), Digest: record.Provider.Digest.String(),
		Contribution: record.Contribution.String(), Contract: string(record.Contract.JSON()),
		Method: record.Method, Input: string(record.Input.JSON()), Target: record.Invocation.Target,
	}, nil
}

func (s *Handler) RetirePluginCommand(ctx context.Context, in protocol.PluginCommandRequest) error {
	if in.Namespace != s.idempotency.Namespace {
		return protocol.ErrIdempotencyStoreMismatch
	}
	if s.pluginCalls == nil {
		return protocol.ErrPluginUnavailable
	}
	identity := operation.Identity(operation.Bind(ctx, s.idempotency.Namespace, operation.ScopedKey(ctx, in.Key)))
	return wirePluginError(s.pluginCalls.RetireCommand(ctx, identity))
}

func (s *Handler) ResolvePluginPoint(ctx context.Context, in protocol.ResolvePluginPointRequest) (*protocol.Page[protocol.PluginPointBinding], error) {
	if s.pluginCalls == nil {
		return nil, NewFailure(protocol.ErrPluginUnavailable, "plugin points are unavailable")
	}
	contract, err := point.Parse([]byte(in.Contract))
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
	values, err := s.pluginCalls.ResolvePoint(ctx, contract, point.Context{Target: in.Target, Attributes: []byte(in.Attributes)}, order)
	if err != nil {
		return nil, wirePluginError(err)
	}
	result := make([]protocol.PluginPointBinding, 0, len(values))
	for _, value := range values {
		result = append(result, protocol.PluginPointBinding{InstallationID: value.Reference.Installation.String(), Digest: value.Digest.String(), Contribution: value.Reference.Contribution.String(), Value: string(value.Value.JSON())})
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
			value.Bindings = append(value.Bindings, protocol.PluginOperationBinding{InstallationID: binding.InstallationID.String(), Digest: binding.Digest.String(), Contribution: binding.ContributionID.String(), Contract: string(binding.Contract.JSON())})
		}
		values = append(values, value)
	}
	return protocol.NewPage(values), nil
}

func pluginCallOperation(in protocol.CallPluginRequest) (OperationKind, error) {
	contract, err := capability.Parse([]byte(in.Contract))
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
	request, err := parsePluginCall(in)
	if err != nil {
		return "", nil, err
	}
	call, err := s.pluginCalls.Prepare(ctx, request)
	if err != nil {
		return "", nil, wirePluginError(err)
	}
	kind := OperationQuery
	if call.Kind() == capability.Mutation {
		kind = OperationCommand
	}
	return kind, func(ctx context.Context) (*protocol.CallPluginResponse, error) {
		output, err := call.Invoke(ctx)
		if failure, known := errors.AsType[*capability.Refusal](err); known && !errors.Is(err, plugin.ErrCommandUnconfirmed) {
			return &protocol.CallPluginResponse{Failure: &protocol.PluginCallFailure{Code: failure.Code, Message: failure.Message}}, nil
		}
		if err != nil {
			return nil, wirePluginError(err)
		}
		return &protocol.CallPluginResponse{Output: string(output.JSON())}, nil
	}, nil
}

func parsePluginCall(in protocol.CallPluginRequest) (plugins.CallRequest, error) {
	id, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return plugins.CallRequest{}, err
	}
	contribution, err := plugin.ParseContributionID(in.Contribution)
	if err != nil {
		return plugins.CallRequest{}, InvalidParameters(err)
	}
	contract, err := capability.Parse([]byte(in.Contract))
	if err != nil {
		return plugins.CallRequest{}, InvalidParameters(err)
	}
	input, err := jsoninput.Parse([]byte(in.Input))
	if err != nil {
		return plugins.CallRequest{}, InvalidParameters(err)
	}
	return plugins.CallRequest{Installation: id, Digest: digest, Contribution: contribution, Contract: contract, Method: in.Method, Input: input, Target: in.Target}, nil
}

func (s *Handler) PreparePluginStream(ctx context.Context, in protocol.CallPluginRequest) (OperationKind, func(context.Context) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error), error) {
	if s.pluginCalls == nil {
		return "", nil, NewFailure(protocol.ErrPluginUnavailable, "plugin streams are unavailable")
	}
	request, err := parsePluginCall(in)
	if err != nil {
		return "", nil, err
	}
	call, err := s.pluginCalls.PrepareStream(ctx, request)
	if err != nil {
		return "", nil, wirePluginError(err)
	}
	kind := OperationQuery
	if call.Kind() == capability.Mutation {
		kind = OperationCommand
	}
	unconfirmed := errors.New("plugin stream final outcome is unconfirmed")
	if call.Kind() == capability.Mutation {
		unconfirmed = errors.Join(plugin.ErrCommandUnconfirmed, unconfirmed)
	}
	return kind, func(ctx context.Context) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error) {
		return struct{}{}, func(yield func(protocol.PluginStreamEvent, error) bool) {
			for value, err := range call.Stream(ctx) {
				if refusal, known := errors.AsType[*capability.Refusal](err); known && !errors.Is(err, plugin.ErrCommandUnconfirmed) {
					yield(protocol.PluginStreamEvent{Kind: protocol.PluginStreamFailure, Failure: &protocol.PluginCallFailure{Code: refusal.Code, Message: refusal.Message}}, nil)
					return
				}
				if err != nil {
					yield(protocol.PluginStreamEvent{}, wirePluginError(err))
					return
				}
				var kind protocol.PluginStreamKind
				switch value.Kind {
				case plugins.StreamChunk:
					kind = protocol.PluginStreamChunk
				case plugins.StreamResult:
					kind = protocol.PluginStreamResult
				default:
					yield(protocol.PluginStreamEvent{}, wirePluginError(unconfirmed))
					return
				}
				if !yield(protocol.PluginStreamEvent{Kind: kind, Value: string(value.Value.JSON())}, nil) {
					return
				}
				if kind == protocol.PluginStreamResult {
					return
				}
			}
			yield(protocol.PluginStreamEvent{}, wirePluginError(unconfirmed))
		}, nil
	}, nil
}

func (s *Handler) NotifyPluginChange(ctx context.Context, notice protocol.PluginChangeNotice) error {
	if s.pluginCalls == nil {
		return NewFailure(protocol.ErrPluginUnavailable, "plugin changes are unavailable")
	}
	contribution, err := plugin.ParseContributionID(notice.Contribution)
	if err != nil {
		return InvalidParameters(err)
	}
	return wirePluginError(s.pluginCalls.NotifyChange(ctx, contribution))
}
