package dispatch

import (
	"context"
	"fmt"
	"iter"
	"reflect"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/transport"
	"github.com/Tangerg/flame/runtime/protocol"
)

// Router adapts JSON-RPC envelopes to the binding-neutral Delivery Endpoint.
// It owns no validation, capability, idempotency or business invocation policy.
type Router struct {
	endpoint *delivery.Endpoint
}

// New builds a Router over the canonical Runtime Delivery Endpoint.
func New(endpoint *delivery.Endpoint) *Router {
	if endpoint == nil {
		panic("dispatch: nil delivery endpoint")
	}
	return &Router{endpoint: endpoint}
}

// Result holds the JSON-RPC reply and optional stream frames for one envelope.
type Result struct {
	Response    *transport.Response
	EventStream iter.Seq[StreamFrame]
}

// Dispatch validates the envelope, decodes its typed parameters and delegates
// the operation itself to the shared endpoint.
func (r *Router) Dispatch(ctx context.Context, message transport.Message) Result {
	request, ok := message.(*transport.Request)
	if !ok || request == nil {
		return responseError(transport.ID{}, badEnvelope("expected a JSON-RPC request"))
	}
	if request.ID.IsValid() {
		if _, ok := request.ID.Raw().(string); !ok {
			return responseError(request.ID, badEnvelope(
				fmt.Sprintf("id must be a JSON string, got %T", request.ID.Raw())))
		}
	}

	requestCopy := *request
	request = &requestCopy
	options := delivery.Options{
		IdempotencyKey:       transport.IdempotencyKeyFrom(ctx),
		IdempotencyNamespace: transport.IdempotencyNamespaceFrom(ctx),
		AfterEventID:         transport.LastEventIDFrom(ctx),
	}
	metadata, metadataError := extractRequestMeta(request)
	if metadataError != nil {
		if !request.IsCall() {
			return Result{}
		}
		return responseError(request.ID, metadataError)
	}
	options.RequestMeta = metadata

	if !request.IsCall() {
		r.handleNotification(ctx, request)
		return Result{}
	}
	name := delivery.Name(request.Method)
	meta, found := delivery.Contract().Lookup(name)
	if !found {
		return responseError(request.ID, errorToRPC(
			delivery.NewFailure(protocol.ErrMethodNotFound, fmt.Sprintf("unknown method %q", request.Method)),
		))
	}
	parameters, decodeError := decodeParameters(request.Params, meta.Params)
	if decodeError != nil {
		return responseError(request.ID, errorToRPC(decodeError))
	}
	return adaptResult(request.ID, r.endpoint.Invoke(ctx, name, parameters, options))
}

func decodeParameters(raw []byte, parameterType reflect.Type) (any, *delivery.Failure) {
	target := reflect.New(parameterType)
	if err := decodeParams(raw, target.Interface()); err != nil {
		return nil, delivery.NewFailure(protocol.ErrInvalidParams, err.Error())
	}
	return target.Elem().Interface(), nil
}

func adaptResult(id transport.ID, result delivery.Result) Result {
	if result.Failure != nil {
		return responseError(id, errorToRPC(result.Failure))
	}
	response := responseResult(id, result.Value)
	if response.Response == nil || response.Response.Error != nil || result.Events == nil {
		return response
	}
	response.EventStream = adaptOperationEvents(result.Events)
	return response
}

func adaptOperationEvents(events iter.Seq2[any, error]) iter.Seq[StreamFrame] {
	return func(yield func(StreamFrame) bool) {
		for event, err := range events {
			if err != nil {
				return
			}
			frame, keep := frameOperationEvent(event)
			if keep && !yield(frame) {
				return
			}
		}
	}
}
