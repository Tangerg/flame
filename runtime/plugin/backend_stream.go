package plugin

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"iter"
	"sync"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/google/uuid"
)

const streamMetadataKey = "io.github.tangerg.flame/stream.v1"

type deliveredChunk struct {
	Sequence uint64         `json:"sequence"`
	Value    jsontext.Value `json:"value"`
}

func chunkDeliveryMethod() (*Method[deliveredChunk, bool], error) {
	return NewMethod[deliveredChunk, bool](MethodConfig{Name: "flame.stream/deliver.v1", Kind: capability.Mutation, Target: "call", Description: "Deliver one ordered chunk to its captured stream consumer."})
}

func (m *StreamingMethod[P, C, R]) RegisterServer(backend *Backend, handler func(context.Context, *sdk.ServerSession, Command[P]) iter.Seq2[StreamValue[C, R], error]) error {
	if m == nil || backend == nil || handler == nil {
		return errors.New("plugin: streaming backend and handler are required")
	}
	return backend.register(bindStreaming(m, handler))
}

func invokeBackendStream(ctx context.Context, session *sdk.ServerSession, method boundMethod, wire *invokeParams) (*Result[jsontext.Value], error) {
	encoded, err := json.Marshal(wire.Meta[streamMetadataKey])
	if err != nil {
		return nil, err
	}
	var ref CallbackRef
	if err := json.Unmarshal(encoded, &ref, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	sequence, err := method.stream(ctx, session, wire)
	if err != nil {
		return nil, err
	}
	delivery, err := chunkDeliveryMethod()
	if err != nil {
		return nil, err
	}
	var index uint64
	for value, err := range sequence {
		if err != nil {
			if refusal, known := errors.AsType[*capability.Refusal](err); known {
				return &Result[jsontext.Value]{Failure: refusal}, nil
			}
			return nil, err
		}
		if value.Kind == "result" {
			return &Result[jsontext.Value]{Value: new(value.Value), valueJSON: value.Value}, nil
		}
		if value.Kind != "chunk" || index == ^uint64(0) {
			return nil, errors.New("plugin: invalid backend stream ordering")
		}
		index++
		ack, err := CallHost(ctx, ref, delivery, uuid.NewString(), deliveredChunk{Sequence: index, Value: value.Value})
		if err != nil {
			return nil, err
		}
		if !ack {
			return nil, errors.New("plugin: stream consumer did not acknowledge chunk")
		}
	}
	return nil, ErrStreamUnconfirmed
}

type pendingChunk struct {
	value    jsontext.Value
	consumed chan struct{}
}
type streamCompletion struct {
	reply *Result[jsontext.Value]
	err   error
}

// StreamWithMetadata retains the original MCP invocation while acknowledged
// callback delivery keeps production behind actual consumer demand.
func (b *Binding) StreamWithMetadata(ctx context.Context, callbacks *Callbacks, limits CallbackLimits, id string, input jsoninput.Input, metadata sdk.ParamsBase) iter.Seq2[StreamValue[jsontext.Value, jsontext.Value], error] {
	frozenMetadata, metadataErr := json.Marshal(metadata)
	return func(yield func(StreamValue[jsontext.Value, jsontext.Value], error) bool) {
		fail := func(err error) {
			yield(StreamValue[jsontext.Value, jsontext.Value]{}, errors.Join(ErrStreamUnconfirmed, err, ctx.Err(), context.Cause(ctx)))
		}
		if b == nil || callbacks == nil || len(b.method.Descriptor().Stream) == 0 {
			fail(errors.New("plugin: streaming binding and callback owner are required"))
			return
		}
		if metadataErr != nil {
			fail(metadataErr)
			return
		}
		var frozen struct {
			Meta map[string]jsontext.Value `json:"_meta,omitzero"`
		}
		if err := json.Unmarshal(frozenMetadata, &frozen, json.RejectUnknownMembers(true)); err != nil {
			fail(err)
			return
		}
		metadata.Meta = sdk.Meta{}
		for key, value := range frozen.Meta {
			metadata.Meta[key] = value
		}
		if err := b.method.ValidateInput(input); err != nil {
			fail(err)
			return
		}
		call, cancel := context.WithCancel(ctx)
		defer cancel()
		chunks := make(chan pendingChunk)
		method, err := chunkDeliveryMethod()
		if err != nil {
			fail(err)
			return
		}
		var mu sync.Mutex
		var received uint64
		receiver, err := NewCallback(method, nil, func(ctx context.Context, command Command[deliveredChunk]) (bool, error) {
			if !mu.TryLock() {
				return false, errors.New("plugin: concurrent stream chunk delivery refused")
			}
			defer mu.Unlock()
			if command.Input.Sequence != received+1 || received == ^uint64(0) {
				return false, errors.New("plugin: stream chunk is not the next original position")
			}
			if err := b.method.ValidateChunk(command.Input.Value); err != nil {
				return false, err
			}
			if int64(len(command.Input.Value)) > limits.MaxResponseBytes {
				return false, errors.New("plugin: stream chunk exceeds response capacity")
			}
			chunk := pendingChunk{value: append(jsontext.Value(nil), command.Input.Value...), consumed: make(chan struct{})}
			select {
			case chunks <- chunk:
			case <-ctx.Done():
				return false, context.Cause(ctx)
			case <-call.Done():
				return false, context.Cause(call)
			}
			select {
			case <-chunk.consumed:
				received++
				return true, nil
			case <-ctx.Done():
				return false, context.Cause(ctx)
			case <-call.Done():
				return false, context.Cause(call)
			}
		})
		if err != nil {
			fail(err)
			return
		}
		ref, release, err := callbacks.Open(call, b.session, limits, receiver)
		if err != nil {
			fail(err)
			return
		}
		defer release()
		copied := sdk.Meta{}
		for key, value := range metadata.Meta {
			copied[key] = value
		}
		copied[streamMetadataKey] = ref
		metadata.Meta = copied
		done := make(chan struct{})
		var completion streamCompletion
		go func() {
			reply, err := b.invokeMode(call, id, input, metadata)
			completion = streamCompletion{reply: reply, err: err}
			close(done)
		}()
		defer func() { cancel(); <-done }()
		for {
			select {
			case chunk := <-chunks:
				if !yield(StreamValue[jsontext.Value, jsontext.Value]{Chunk: new(chunk.value)}, nil) {
					return
				}
				close(chunk.consumed)
			case <-done:
				if completion.err != nil {
					if refusal, known := errors.AsType[*capability.Refusal](completion.err); known {
						yield(StreamValue[jsontext.Value, jsontext.Value]{}, refusal)
						return
					}
					fail(completion.err)
					return
				}
				if completion.reply == nil {
					fail(ErrStreamUnconfirmed)
					return
				}
				if int64(len(completion.reply.valueJSON)) > limits.MaxResponseBytes {
					fail(errors.New("plugin: stream final result exceeds response capacity"))
					return
				}
				yield(StreamValue[jsontext.Value, jsontext.Value]{Result: new(completion.reply.valueJSON)}, nil)
				return
			case <-call.Done():
				fail(context.Cause(call))
				return
			}
		}
	}
}

func (m *StreamingMethod[P, C, R]) StreamWithMetadata(ctx context.Context, binding *Binding, callbacks *Callbacks, limits CallbackLimits, id string, input P, metadata sdk.ParamsBase) iter.Seq2[StreamValue[C, R], error] {
	admitted, admissionErr := jsoninput.Encode(input)
	source := binding.StreamWithMetadata(ctx, callbacks, limits, id, admitted, metadata)
	return func(yield func(StreamValue[C, R], error) bool) {
		if admissionErr != nil || m == nil || binding == nil || m.contract.Identity() != binding.Contract() {
			yield(StreamValue[C, R]{}, errors.Join(admissionErr, errors.New("plugin: typed stream differs from captured binding")))
			return
		}
		for raw, err := range source {
			if err != nil {
				yield(StreamValue[C, R]{}, err)
				return
			}
			var value StreamValue[C, R]
			if raw.Chunk != nil {
				value.Chunk = new(C)
				err = json.Unmarshal(*raw.Chunk, value.Chunk, json.RejectUnknownMembers(true))
			} else {
				value.Result = new(R)
				err = json.Unmarshal(*raw.Result, value.Result, json.RejectUnknownMembers(true))
			}
			if err != nil {
				yield(StreamValue[C, R]{}, errors.Join(ErrStreamUnconfirmed, err))
				return
			}
			if !yield(value, nil) {
				return
			}
		}
	}
}
