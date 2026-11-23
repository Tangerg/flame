package client

import (
	"bytes"
	"context"
	"iter"
	"reflect"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/Tangerg/frame"
)

type methodMeta struct {
	Name                    string
	Kind                    methodKind
	Operation               operation
	Result                  reflect.Type
	Event                   reflect.Type
	ResultNullable          bool
	AcknowledgementMismatch func(parameters, result any) string
}

func lookupMethod(name string) (methodMeta, bool) { meta, ok := methodMetadata[name]; return meta, ok }

func (m methodMeta) decodeResult(encoded []byte) (any, error) {
	if m.ResultNullable && bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
		return reflect.Zero(m.Result).Interface(), nil
	}
	target := reflect.New(m.Result)
	if err := protocol.DecodeResponse(encoded, target.Interface()); err != nil {
		return nil, err
	}
	return target.Elem().Interface(), nil
}

type methodKind uint8

const (
	unaryMethod methodKind = iota + 1
	streamMethod
)

type operation uint8

const (
	queryOperation operation = iota + 1
	commandOperation
	subscriptionOperation
	extensionOperation
)

func invocationOperation(meta methodMeta, request any) (operation, error) {
	if meta.Operation != extensionOperation {
		return meta.Operation, nil
	}
	call, ok := request.(protocol.CallPluginRequest)
	if !ok {
		return 0, protocol.ErrInvalidParams
	}
	kind, err := plugincontract.MethodKindFor([]byte(call.Contract), call.Method)
	if err != nil {
		return 0, protocol.InvalidParameters(err)
	}
	if kind == frame.Command {
		return commandOperation, nil
	}
	return queryOperation, nil
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
	options protocol.CallMetadata,
) (Response, error) {
	var zero Response
	if r == nil {
		return zero, ErrClosed
	}
	result, err := r.invokeOperation(ctx, name, request, options)
	if err != nil {
		return zero, err
	}
	return result.Response[Response](name)
}

func (r *Client) invokeAck[Request any](
	ctx context.Context,
	name string,
	request Request,
	options protocol.CallMetadata,
) error {
	_, err := r.invoke[Request, struct{}](ctx, name, request, options)
	return err
}

func (r *Client) invokeStream[Request, Ack, Event any](
	ctx context.Context,
	name string,
	request Request,
	options protocol.CallMetadata,
) (Ack, iter.Seq2[Event, error], error) {
	var zero Ack
	if r == nil {
		return zero, nil, ErrClosed
	}
	result, err := r.invokeOperation(ctx, name, request, options)
	if err != nil {
		return zero, nil, err
	}
	return result.Stream[Ack, Event](name)
}
