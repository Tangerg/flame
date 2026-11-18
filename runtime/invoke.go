package runtime

import (
	"context"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

// binding owns the typed Go projection for both deployment modes. Each caller
// crosses its real delivery boundary before returning catalog-owned values.
type binding struct {
	caller operationCaller
}

type operationCaller interface {
	invokeOperation(context.Context, delivery.Name, any, protocol.CallMetadata) (protocol.OperationResult, error)
}

func (r *Runtime) invokeOperation(ctx context.Context, name delivery.Name, request any, options protocol.CallMetadata) (protocol.OperationResult, error) {
	endpoint, err := r.endpoint()
	if err != nil {
		return protocol.OperationResult{}, err
	}
	return endpoint.Invoke(ctx, name, request, options), nil
}

func (r *binding) invoke[Request, Response any](
	ctx context.Context,
	name delivery.Name,
	request Request,
	options protocol.CallMetadata,
) (Response, error) {
	var zero Response
	if r == nil || r.caller == nil {
		return zero, ErrClosed
	}
	result, err := r.caller.invokeOperation(ctx, name, request, options)
	if err != nil {
		return zero, err
	}
	return result.Response[Response](string(name))
}

func (r *binding) invokeAck[Request any](
	ctx context.Context,
	name delivery.Name,
	request Request,
	options protocol.CallMetadata,
) error {
	_, err := r.invoke[Request, struct{}](ctx, name, request, options)
	return err
}

func (r *binding) invokeStream[Request, Ack, Event any](
	ctx context.Context,
	name delivery.Name,
	request Request,
	options protocol.CallMetadata,
) (Ack, iter.Seq2[Event, error], error) {
	var zero Ack
	if r == nil || r.caller == nil {
		return zero, nil, ErrClosed
	}
	result, err := r.caller.invokeOperation(ctx, name, request, options)
	if err != nil {
		return zero, nil, err
	}
	return result.Stream[Ack, Event](string(name))
}
