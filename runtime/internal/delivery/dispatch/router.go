package dispatch

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/go-sdk/jsonrpc"
)

type Router struct {
	endpoint *delivery.Endpoint
}

func New(endpoint *delivery.Endpoint) (*Router, error) {
	if endpoint == nil {
		return nil, errors.New("dispatch: delivery endpoint is required")
	}
	return &Router{endpoint: endpoint}, nil
}

type Result struct {
	Response    *jsonrpc.Response
	EventStream iter.Seq[StreamFrame]
}

func (r *Router) Dispatch(ctx context.Context, message jsonrpc.Message, options protocol.CallMetadata) Result {
	request, ok := message.(*jsonrpc.Request)
	if !ok || request == nil {
		return responseError(jsonrpc.ID{}, badEnvelope("expected a JSON-RPC request"))
	}
	if request.ID.IsValid() {
		if _, ok := request.ID.Raw().(string); !ok {
			return responseError(request.ID, badEnvelope(
				fmt.Sprintf("id must be a JSON string, got %T", request.ID.Raw())))
		}
	}

	requestCopy := *request
	request = &requestCopy
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
	return adaptResult(request.ID, r.endpoint.InvokeWire(ctx, name, request.Params, options))
}

func adaptResult(id jsonrpc.ID, result protocol.OperationResult) Result {
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

// Skipping a frame would hand the client a complete-looking stream with a hole;
// ending it lets Last-Event-Id replay the gap.
func adaptOperationEvents(events iter.Seq2[any, error]) iter.Seq[StreamFrame] {
	return func(yield func(StreamFrame) bool) {
		for event, err := range events {
			if err != nil {
				return
			}
			frame, publishable := frameOperationEvent(event)
			if !publishable || !yield(frame) {
				return
			}
		}
	}
}
