package plugin

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"net/http"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/jsonschema"
	"github.com/Tangerg/sse"
)

var ErrStreamUnconfirmed = errors.New("plugin: stream final outcome is unconfirmed")

type StreamValue[C, R any] struct {
	Chunk  *C
	Result *R
}

type StreamingMethod[P, C, R any] struct{ contract *capability.Method }

func NewStreamingMethod[P, C, R any](config MethodConfig) (*StreamingMethod[P, C, R], error) {
	base, err := NewMethod[P, R](config)
	if err != nil {
		return nil, err
	}
	chunk, err := jsonschema.For[C]()
	if err != nil {
		return nil, err
	}
	descriptor := base.Descriptor()
	descriptor.Stream = chunk.JSON()
	contract, err := capability.AdmitMethod(descriptor)
	if err != nil {
		return nil, err
	}
	return &StreamingMethod[P, C, R]{contract: contract}, nil
}

func (m *StreamingMethod[P, C, R]) Descriptor() capability.MethodDescriptor {
	return m.contract.Descriptor()
}

type streamValue struct {
	Kind    string         `json:"kind"`
	Value   jsontext.Value `json:"value,omitzero"`
	Message string         `json:"message,omitzero"`
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
	return boundMethod{contract: method.contract, stream: func(ctx context.Context, session *sdk.ServerSession, wire *invokeParams) (iter.Seq2[streamValue, error], error) {
		ctx, command, err := admitCommand[P](ctx, method.contract, wire)
		if err != nil {
			return nil, err
		}
		return func(yield func(streamValue, error) bool) {
			final := false
			var outcome streamValue
			for item, err := range handler(ctx, session, command) {
				if err != nil {
					yield(streamValue{}, err)
					return
				}
				if final || (item.Chunk == nil) == (item.Result == nil) {
					yield(streamValue{}, errors.New("plugin: invalid stream value ordering"))
					return
				}
				frame := streamValue{Kind: "chunk"}
				var data []byte
				if item.Chunk != nil {
					data, err = json.Marshal(item.Chunk)
					if err == nil {
						err = method.contract.ValidateChunk(data)
					}
				} else {
					frame.Kind, final = "result", true
					data, err = json.Marshal(item.Result)
					if err == nil {
						err = method.contract.ValidateOutput(data)
					}
				}
				if err != nil {
					yield(streamValue{}, err)
					return
				}
				frame.Value = data
				if final {
					outcome = frame
					continue
				}
				if !yield(frame, nil) {
					return
				}
			}
			if !final {
				yield(streamValue{}, ErrStreamUnconfirmed)
			} else {
				yield(outcome, nil)
			}
		}, nil
	}}
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
			frame = streamValue{Kind: "error", Message: err.Error()}
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

// StreamHost starts one delivery only when consumed. Closing iteration cancels it;
// transport writes and reads provide bounded backpressure without an event queue.
func StreamHost[P, C, R any](ctx context.Context, ref CallbackRef, method *StreamingMethod[P, C, R], id string, input P) iter.Seq2[StreamValue[C, R], error] {
	admitted, admissionErr := jsoninput.Encode(input)
	return func(yield func(StreamValue[C, R], error) bool) {
		fail := func(err error) {
			yield(StreamValue[C, R]{}, errors.Join(ErrStreamUnconfirmed, err, ctx.Err(), context.Cause(ctx)))
		}
		if err := ref.Validate(); err != nil {
			fail(err)
			return
		}
		if ref.MaxResponseBytes > int64(int(^uint(0)>>1))-int64(len("data: ")) {
			fail(errors.New("plugin: callback frame capacity is invalid"))
			return
		}
		if method == nil {
			fail(errors.New("plugin: streaming method is required"))
			return
		}
		if method.contract.Kind() == capability.Mutation {
			if err := validateCommandID(id); err != nil {
				fail(err)
				return
			}
		} else if id != "" {
			fail(errors.New("plugin: query must not carry command identity"))
			return
		}
		err := admissionErr
		if err == nil {
			err = method.contract.ValidateInput(admitted)
		}
		if err != nil {
			fail(err)
			return
		}
		encoded, err := json.Marshal(invokeParams{Protocol: capability.Protocol, Method: method.contract.Name(), Contract: method.contract.Identity().Digest(), Input: admitted.JSON(), CommandID: id})
		if err != nil {
			fail(err)
			return
		}
		call, cancel := context.WithCancel(ctx)
		defer cancel()
		request, err := http.NewRequestWithContext(call, http.MethodPost, ref.Endpoint, bytes.NewReader(encoded))
		if err != nil {
			fail(err)
			return
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "text/event-stream")
		request.Header.Set("Authorization", ref.Credential)
		request.Header.Set("Flame-Call", ref.Call)
		if err := setCallbackInvocation(request, ctx); err != nil {
			fail(err)
			return
		}
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		response, err := client.Do(request)
		if err != nil {
			fail(err)
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			fail(fmt.Errorf("plugin: callback stream could not be confirmed (%d)", response.StatusCode))
			return
		}
		reader, err := sse.NewHTTPReader(response)
		if err != nil {
			fail(err)
			return
		}
		reader.MaxEventBytes, reader.MaxLineBytes = int(ref.MaxResponseBytes), int(ref.MaxResponseBytes)+len("data: ")
		final := false
		for message, err := range reader.Messages() {
			if err != nil {
				fail(err)
				return
			}
			var frame streamValue
			if err := json.Unmarshal(message.Data, &frame, json.RejectUnknownMembers(true)); err != nil {
				fail(err)
				return
			}
			if final {
				fail(errors.New("plugin: data after stream result"))
				return
			}
			if frame.Kind == "error" && (len(frame.Value) != 0 || frame.Message == "") || frame.Kind != "error" && frame.Message != "" {
				fail(errors.New("plugin: invalid stream frame fields"))
				return
			}
			var item StreamValue[C, R]
			switch frame.Kind {
			case "chunk":
				err = method.contract.ValidateChunk(frame.Value)
				if err == nil {
					item.Chunk = new(C)
					err = json.Unmarshal(frame.Value, item.Chunk, json.RejectUnknownMembers(true))
				}
			case "result":
				err = method.contract.ValidateOutput(frame.Value)
				if err == nil {
					item.Result = new(R)
					err = json.Unmarshal(frame.Value, item.Result, json.RejectUnknownMembers(true))
				}
				final = true
			case "error":
				err = errors.New(frame.Message)
			default:
				err = errors.New("plugin: unknown stream frame")
			}
			if err != nil {
				fail(err)
				return
			}
			if !yield(item, nil) {
				return
			}
		}
		if !final {
			fail(ErrStreamUnconfirmed)
		}
	}
}
