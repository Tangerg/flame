package mcp

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"reflect"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"

	"github.com/Tangerg/frame"
	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/sse"
)

var ErrStreamUnconfirmed = errors.New("plugin: stream final outcome is unconfirmed")

// A stream yields chunks and then exactly one result. The constructors keep
// every value one or the other; only the zero value, which accompanies an
// error, carries neither.
type StreamValue[C, R any] struct {
	chunk  *C
	result *R
}

func StreamChunk[R, C any](chunk C) StreamValue[C, R] {
	return StreamValue[C, R]{chunk: &chunk}
}

func StreamResult[C, R any](result R) StreamValue[C, R] {
	return StreamValue[C, R]{result: &result}
}

func (v StreamValue[C, R]) Chunk() (C, bool) {
	if v.chunk == nil {
		var zero C
		return zero, false
	}
	return *v.chunk, true
}

func (v StreamValue[C, R]) Result() (R, bool) {
	if v.result == nil {
		var zero R
		return zero, false
	}
	return *v.result, true
}

type StreamingMethod[P, C, R any] struct{ contract *frame.Method }

func ImplementStreamingMethod[P, C, R any](owner *plugincontract.Capability, name string) (*StreamingMethod[P, C, R], error) {
	if owner == nil {
		return nil, errors.New("plugin: method contract is required")
	}
	method, ok := owner.Method(name)
	if !ok {
		return nil, fmt.Errorf("plugin: contract does not publish method %q", name)
	}
	if len(method.Descriptor().Stream) == 0 {
		return nil, errors.New("plugin: unary contract requires a unary implementation")
	}
	if err := validateMethodCodecs[P, R](name); err != nil {
		return nil, err
	}
	if err := framevalue.ValidateType(reflect.TypeFor[C]()); err != nil {
		return nil, fmt.Errorf("plugin: %s chunk codec: %w", name, err)
	}
	return &StreamingMethod[P, C, R]{contract: method}, nil
}

func NewStreamingMethod[P, C, R any](config frame.MethodConfig) (*StreamingMethod[P, C, R], error) {
	if _, err := NewMethod[P, R](config); err != nil {
		return nil, err
	}
	contract, err := frame.NewStreamingMethod[P, C, R](config)
	if err != nil {
		return nil, err
	}

	return &StreamingMethod[P, C, R]{contract: contract}, nil
}

func (m *StreamingMethod[P, C, R]) Descriptor() frame.MethodDescriptor {
	return m.contract.Descriptor()
}

type streamFrameKind string

const (
	streamChunk  streamFrameKind = "chunk"
	streamResult streamFrameKind = "result"
	streamError  streamFrameKind = "error"
)

type streamValue struct {
	Kind    streamFrameKind `json:"kind"`
	Value   jsontext.Value  `json:"value,omitzero"`
	Message string          `json:"message,omitzero"`
}

type streamCapacityKey struct{}

func NewStreamingCallback[P, C, R any](method *StreamingMethod[P, C, R], handler func(context.Context, Command[P]) iter.Seq2[StreamValue[C, R], error]) (Callback, error) {
	if method == nil || handler == nil {
		return Callback{}, errors.New("plugin: streaming callback requires its method and owner")
	}
	return Callback{method: bindStreaming(method, func(ctx context.Context, _ *sdk.ServerSession, command Command[P]) iter.Seq2[StreamValue[C, R], error] {
		return handler(ctx, command)
	})}, nil
}

func bindStreaming[P, C, R any](method *StreamingMethod[P, C, R], handler func(context.Context, *sdk.ServerSession, Command[P]) iter.Seq2[StreamValue[C, R], error]) boundMethod {
	contract := method.contract
	return boundMethod{contract: contract, validate: validateMethodInput[P](contract), stream: func(ctx context.Context, session *sdk.ServerSession, wire *invokeParams) (iter.Seq2[streamValue, error], error) {
		ctx, command, err := admitCommand[P](ctx, contract, wire)
		if err != nil {
			return nil, err
		}
		return func(yield func(streamValue, error) bool) {
			var outcome *streamValue
			for item, err := range handler(ctx, session, command) {
				if err == nil && outcome != nil {
					err = errors.New("plugin: stream value after its result")
				}
				var frame streamValue
				if err == nil {
					frame, err = encodeStreamFrame(contract, item)
				}
				if err != nil {
					yield(streamValue{}, err)
					return
				}
				if frame.Kind == streamResult {
					outcome = &frame
					continue
				}
				if !yield(frame, nil) {
					return
				}
			}
			if outcome == nil {
				yield(streamValue{}, ErrStreamUnconfirmed)
				return
			}
			yield(*outcome, nil)
		}, nil
	}}
}

func encodeStreamFrame[C, R any](contract *frame.Method, item StreamValue[C, R]) (streamValue, error) {
	switch {
	case item.chunk != nil:
		data, err := json.Marshal(item.chunk)
		if err == nil {
			err = contract.ValidateChunk(data)
		}
		return streamValue{Kind: streamChunk, Value: data}, err
	case item.result != nil:
		data, err := json.Marshal(item.result)
		if err == nil {
			err = contract.ValidateOutput(data)
		}
		return streamValue{Kind: streamResult, Value: data}, err
	default:
		return streamValue{}, errors.New("plugin: stream value carries neither a chunk nor a result")
	}
}

func serveCallbackStream(w http.ResponseWriter, r *http.Request, method boundMethod, wire *invokeParams, limits CallbackLimits) {
	sequence, err := method.stream(context.WithValue(r.Context(), streamCapacityKey{}, limits.MaxResponseBytes), nil, wire)
	if err != nil {
		http.Error(w, "callback stream input refused", http.StatusBadRequest)
		return
	}
	writer := sse.NewHTTPWriter(w)
	for frame, err := range sequence {
		if err != nil {
			frame = streamValue{Kind: streamError, Message: err.Error()}
		}
		data, encodeErr := json.Marshal(frame)
		if encodeErr != nil || int64(len(data)) > limits.MaxResponseBytes {
			return
		}
		if r.Context().Err() != nil || writer.Write(sse.Message{Data: data}) != nil {
			return
		}
		if err != nil {
			return
		}
	}
}

const (
	eventStreamMediaType = "text/event-stream"
	jsonMediaType        = "application/json"
	jsonNull             = "null"
	headerAuthorization  = "Authorization"
	headerContentType    = "Content-Type"
	headerAccept         = "Accept"
	sseDataPrefix        = "data: "
)

func StreamHost[P, C, R any](ctx context.Context, ref CallbackRef, method *StreamingMethod[P, C, R], id CommandID, input P) iter.Seq2[StreamValue[C, R], error] {
	if method != nil {
		method = new(*method)
	}
	admitted, admissionErr := framevalue.EncodeJSON(input)
	return func(yield func(StreamValue[C, R], error) bool) {
		fail := func(err error) {
			yield(StreamValue[C, R]{}, errors.Join(ErrStreamUnconfirmed, err, ctx.Err(), context.Cause(ctx)))
		}
		if method == nil {
			fail(errors.New("plugin: streaming method is required"))
			return
		}
		if err := errors.Join(validateStreamRef(ref, method.contract, id), admissionErr); err != nil {
			fail(err)
			return
		}
		call, cancel := context.WithCancel(ctx)
		defer cancel()
		reader, body, err := openHostStream(call, ref, method.contract, id, admitted)
		if err != nil {
			fail(err)
			return
		}
		defer body.Close()
		final := false
		for message, err := range reader.Messages() {
			if err == nil && final {
				err = errors.New("plugin: data after stream result")
			}
			var item StreamValue[C, R]
			if err == nil {
				item, err = decodeStreamFrame[C, R](method.contract, message.Data)
			}
			if err != nil {
				fail(err)
				return
			}
			final = item.result != nil
			if !yield(item, nil) {
				return
			}
		}
		if !final {
			fail(ErrStreamUnconfirmed)
		}
	}
}

func validateStreamRef(ref CallbackRef, contract *frame.Method, id CommandID) error {
	if err := validateHostCall(ref, contract, id); err != nil {
		return err
	}
	if ref.MaxResponseBytes > int64(int(^uint(0)>>1))-int64(len(sseDataPrefix)) {
		return errors.New("plugin: callback frame capacity is invalid")
	}
	return nil
}

func openHostStream(call context.Context, ref CallbackRef, contract *frame.Method, id CommandID, admitted framevalue.JSON) (*sse.Reader, io.Closer, error) {
	request, err := newHostRequest(call, ref, contract, id, admitted)
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set(headerAccept, eventStreamMediaType)
	response, err := callbackClient.Do(request)
	if err != nil {
		return nil, nil, err
	}
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusTooManyRequests:
		return nil, nil, errors.Join(ErrCallbackCapacity, response.Body.Close())
	default:
		return nil, nil, errors.Join(fmt.Errorf("plugin: callback stream could not be confirmed (%d)", response.StatusCode), response.Body.Close())
	}
	reader, err := sse.NewHTTPReader(response)
	if err != nil {
		return nil, nil, errors.Join(err, response.Body.Close())
	}
	reader.MaxEventBytes, reader.MaxLineBytes = int(ref.MaxResponseBytes), int(ref.MaxResponseBytes)+len(sseDataPrefix)
	return reader, response.Body, nil
}

func decodeStreamFrame[C, R any](method *frame.Method, data []byte) (StreamValue[C, R], error) {
	var frame streamValue
	if err := json.Unmarshal(data, &frame, json.RejectUnknownMembers(true)); err != nil {
		return StreamValue[C, R]{}, err
	}
	if frame.Kind == streamError && (len(frame.Value) != 0 || frame.Message == "") || frame.Kind != streamError && frame.Message != "" {
		return StreamValue[C, R]{}, errors.New("plugin: invalid stream frame fields")
	}
	var item StreamValue[C, R]
	switch frame.Kind {
	case streamChunk:
		if err := method.ValidateChunk(frame.Value); err != nil {
			return item, err
		}
		item.chunk = new(C)
		if err := json.Unmarshal(frame.Value, item.chunk, json.RejectUnknownMembers(true), framevalue.Numbers()); err != nil {
			return StreamValue[C, R]{}, err
		}
	case streamResult:
		if err := method.ValidateOutput(frame.Value); err != nil {
			return item, err
		}
		item.result = new(R)
		if err := json.Unmarshal(frame.Value, item.result, json.RejectUnknownMembers(true), framevalue.Numbers()); err != nil {
			return StreamValue[C, R]{}, err
		}
	case streamError:
		return item, errors.New(frame.Message)
	default:
		return item, errors.New("plugin: unknown stream frame")
	}
	return item, nil
}
