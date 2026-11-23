package runtime

import (
	"context"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func (r *Runtime) invokeOperation(ctx context.Context, name delivery.Name, request any, options protocol.CallMetadata) (protocol.OperationResult, error) {
	endpoint, err := r.endpoint()
	if err != nil {
		return protocol.OperationResult{}, err
	}
	return endpoint.Invoke(ctx, name, request, options), nil
}

func (r *Runtime) invoke[Request, Response any](
	ctx context.Context,
	name delivery.Name,
	request Request,
	options protocol.CallMetadata,
) (Response, error) {
	result, err := r.invokeOperation(ctx, name, request, options)
	if err != nil {
		var zero Response
		return zero, err
	}
	return result.Response[Response](string(name))
}

func (r *Runtime) invokeAck[Request any](
	ctx context.Context,
	name delivery.Name,
	request Request,
	options protocol.CallMetadata,
) error {
	_, err := r.invoke[Request, struct{}](ctx, name, request, options)
	return err
}

func (r *Runtime) invokeStream[Request, Ack, Event any](
	ctx context.Context,
	name delivery.Name,
	request Request,
	options protocol.CallMetadata,
) (Ack, iter.Seq2[Event, error], error) {
	result, err := r.invokeOperation(ctx, name, request, options)
	if err != nil {
		var zero Ack
		return zero, nil, err
	}
	return result.Stream[Ack, Event](string(name))
}
