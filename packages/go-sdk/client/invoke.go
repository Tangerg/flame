package client

import (
	"bytes"
	"context"
	"fmt"
	"iter"
	"reflect"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type callResult struct {
	Value   any
	Failure *protocol.Failure
	Events  iter.Seq2[any, error]
}
type methodMeta struct {
	Name           string
	Kind           string
	Operation      string
	Result         reflect.Type
	Event          reflect.Type
	ResultNullable bool
}

func lookupMethod(name string) (methodMeta, bool) { meta, ok := methodMetadata[name]; return meta, ok }
func (m methodMeta) DecodeResult(encoded []byte) (any, error) {
	if m.ResultNullable && bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
		return reflect.Zero(m.Result).Interface(), nil
	}
	target := reflect.New(m.Result)
	if err := protocol.DecodeResponse(encoded, target.Interface()); err != nil {
		return nil, err
	}
	return target.Elem().Interface(), nil
}
func valueAs[T any](r callResult) (T, bool) {
	var zero T
	if r.Value == nil && reflect.TypeFor[T]().Kind() == reflect.Interface {
		return zero, true
	}
	value, ok := r.Value.(T)
	return value, ok
}
func invocationOperation(meta methodMeta, request any) (string, error) {
	if meta.Operation != "extension" {
		return meta.Operation, nil
	}
	call, ok := request.(protocol.CallPluginRequest)
	if !ok {
		return "", protocol.ErrInvalidParams
	}
	kind, err := capability.KindFor([]byte(call.Contract), call.Method)
	if err != nil {
		// A malformed contract or an undeclared method is a caller error; project
		// it to the public invalid_params identity the local handler also uses,
		// before any transport, so errors.Is/As stay recognizable to callers.
		return "", protocol.InvalidParameters(err)
	}
	if kind == capability.Mutation {
		return "command", nil
	}
	return "query", nil
}

type transportProblem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail"`
	RequestID string `json:"requestId,omitempty"`
}

func (r *Client) invoke[Request, Response any](
	ctx context.Context,
	name string,
	request Request,
	options callConfig,
) (Response, error) {
	var zero Response
	if r == nil {
		return zero, ErrClosed
	}
	result, err := r.invokeOperation(ctx, name, request, options)
	if err != nil {
		return zero, err
	}
	if result.Failure != nil {
		return zero, result.Failure
	}
	value, ok := valueAs[Response](result)
	if !ok {
		return zero, fmt.Errorf("runtime: %s returned an invalid response type: %w", name, protocol.ErrInvalidResponse)
	}
	return value, nil
}

func (r *Client) invokeAck[Request any](
	ctx context.Context,
	name string,
	request Request,
	options callConfig,
) error {
	_, err := r.invoke[Request, struct{}](ctx, name, request, options)
	return err
}

func (r *Client) invokeStream[Request, Ack, Event any](
	ctx context.Context,
	name string,
	request Request,
	options callConfig,
) (Ack, iter.Seq2[Event, error], error) {
	var zero Ack
	if r == nil {
		return zero, nil, ErrClosed
	}
	result, err := r.invokeOperation(ctx, name, request, options)
	if err != nil {
		return zero, nil, err
	}
	if result.Failure != nil {
		return zero, nil, result.Failure
	}
	ack, ok := valueAs[Ack](result)
	if !ok || result.Events == nil {
		return zero, nil, fmt.Errorf("runtime: %s returned an invalid stream: %w", name, protocol.ErrInvalidResponse)
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
				yield(zero, fmt.Errorf("runtime: %s returned an invalid event type: %w", name, protocol.ErrInvalidResponse))
				return
			}
			if !yield(event, nil) {
				return
			}
		}
	}, nil
}
