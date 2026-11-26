package dispatch

import (
	"context"
	"errors"
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

type Reply struct {
	Response    jsonrpc.Response
	EventStream iter.Seq[StreamFrame]
}

func (r *Router) Call(ctx context.Context, request *jsonrpc.Request, options protocol.CallMetadata) Reply {
	parameters, metadata, metadataError := splitRequestMetadata(request.Params)
	if metadataError != nil {
		return errorReply(request.ID, metadataError)
	}
	options.RequestMeta = metadata
	name := delivery.Name(request.Method)
	return adaptResult(request.ID, r.endpoint.InvokeWire(ctx, name, parameters, options))
}

func adaptResult(id jsonrpc.ID, result protocol.OperationResult) Reply {
	if result.Failure != nil {
		return errorReply(id, errorToRPC(result.Failure))
	}
	response, err := protocol.NewRPCResult(id, result.Value)
	if err != nil {
		return errorReply(id, problemError(protocol.ErrInternalError, "the runtime could not encode the response"))
	}
	reply := Reply{Response: *response}
	if result.Events != nil {
		reply.EventStream = adaptOperationEvents(result.Events)
	}
	return reply
}

func errorReply(id jsonrpc.ID, failure *jsonrpc.Error) Reply {
	return Reply{Response: *protocol.NewRPCErrorResponse(id, failure)}
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
