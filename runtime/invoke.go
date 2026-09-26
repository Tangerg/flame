package runtime

import (
	"context"
	"fmt"
	"iter"

	"github.com/Tangerg/flame/runtime/internal/delivery"
)

// binding owns the typed Go projection for both deployment modes. Each caller
// crosses its real delivery boundary before returning catalog-owned values.
type binding struct {
	caller operationCaller
}

type operationCaller interface {
	invokeOperation(context.Context, delivery.Name, any, delivery.Options) (delivery.Result, error)
}

func (r *Runtime) invokeOperation(ctx context.Context, name delivery.Name, request any, options delivery.Options) (delivery.Result, error) {
	endpoint, err := r.endpoint()
	if err != nil {
		return delivery.Result{}, err
	}
	return endpoint.Invoke(ctx, name, request, options), nil
}

func (r *binding) invoke[Request, Response any](
	ctx context.Context,
	name delivery.Name,
	request Request,
	options delivery.Options,
) (Response, error) {
	var zero Response
	if r == nil || r.caller == nil {
		return zero, ErrClosed
	}
	result, err := r.caller.invokeOperation(ctx, name, request, options)
	if err != nil {
		return zero, err
	}
	if result.Failure != nil {
		return zero, result.Failure
	}
	value, ok := result.Value.(Response)
	if !ok {
		return zero, fmt.Errorf("runtime: %s returned an invalid response type: %w", name, ErrInvalidResponse)
	}
	return value, nil
}

func (r *binding) invokeAck[Request any](
	ctx context.Context,
	name delivery.Name,
	request Request,
	options delivery.Options,
) error {
	_, err := r.invoke[Request, struct{}](ctx, name, request, options)
	return err
}

func (r *binding) invokeStream[Request, Ack, Event any](
	ctx context.Context,
	name delivery.Name,
	request Request,
	options delivery.Options,
) (Ack, iter.Seq2[Event, error], error) {
	var zero Ack
	if r == nil || r.caller == nil {
		return zero, nil, ErrClosed
	}
	result, err := r.caller.invokeOperation(ctx, name, request, options)
	if err != nil {
		return zero, nil, err
	}
	if result.Failure != nil {
		return zero, nil, result.Failure
	}
	ack, ok := result.Value.(Ack)
	if !ok || result.Events == nil {
		return zero, nil, fmt.Errorf("runtime: %s returned an invalid stream: %w", name, ErrInvalidResponse)
	}
	return ack, func(yield func(Event, error) bool) {
		var zero Event
		for value, err := range result.Events {
			if err != nil {
				yield(zero, err)
				return
			}
			event, ok := value.(Event)
			if !ok {
				yield(zero, fmt.Errorf("runtime: %s returned an invalid event type: %w", name, ErrInvalidResponse))
				return
			}
			if !yield(event, nil) {
				return
			}
		}
	}, nil
}
