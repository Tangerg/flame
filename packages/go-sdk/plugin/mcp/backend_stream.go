package mcp

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"iter"
	"sync"
	"time"

	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"

	sdk "github.com/Tangerg/go-sdk/mcp"

	scopemetadata "github.com/Tangerg/scope/core/metadata"
)

// A cooperative backend can settle its original outcome after a negative chunk
// acknowledgement. A peer that ignores it cannot hold the consumer indefinitely.
const streamStopGrace = 2 * time.Second

const StreamMetadataKey = "io.github.tangerg.flame/stream.v1"

type deliveredChunk struct {
	Sequence uint64         `json:"sequence"`
	Value    jsontext.Value `json:"value"`
}

func chunkDeliveryMethod() (*Method[deliveredChunk, bool], error) {
	return NewMethod[deliveredChunk, bool](portable.MethodConfig{Name: "flame.stream/deliver.v1", Kind: portable.Command, Target: "call", Description: "Deliver one ordered chunk to its captured stream consumer."})
}

// StreamDeliveryContract projects the existing acknowledged chunk callback for
// independent language bindings. Its schema identity remains owned here.
func StreamDeliveryContract() (portable.MethodDescriptor, error) {
	method, err := chunkDeliveryMethod()
	if err != nil {
		return portable.MethodDescriptor{}, err
	}
	return method.Descriptor(), nil
}

func (m *StreamingMethod[P, C, R]) RegisterServer(backend *Backend, handler func(context.Context, *sdk.ServerSession, Command[P]) iter.Seq2[StreamValue[C, R], error]) error {
	if m == nil || backend == nil || handler == nil {
		return errors.New("plugin: streaming backend and handler are required")
	}
	return backend.register(bindStreaming(m, handler))
}

func invokeBackendStream(ctx context.Context, session *sdk.ServerSession, method boundMethod, wire *invokeParams) (*methodResult[jsontext.Value], error) {
	var ref CallbackRef
	if err := json.Unmarshal(wire.Meta[StreamMetadataKey], &ref, json.RejectUnknownMembers(true)); err != nil {
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
			if refusal, known := errors.AsType[*framemcp.Refusal](err); known {
				return &methodResult[jsontext.Value]{Failure: refusal}, nil
			}
			return nil, err
		}
		if value.Kind == "result" {
			return &methodResult[jsontext.Value]{Value: new(value.Value), valueJSON: value.Value}, nil
		}
		if value.Kind != "chunk" || index == ^uint64(0) {
			return nil, errors.New("plugin: invalid backend stream ordering")
		}
		index++
		ack, err := CallHost(ctx, ref, delivery, NewCommandID(), deliveredChunk{Sequence: index, Value: value.Value})
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
	value        jsontext.Value
	acknowledged chan bool
}

// A negative acknowledgement must reach the producer before cancellation so
// that its iterator can finish cleanup and publish the original receipt.
func (chunk pendingChunk) acknowledge(ctx context.Context, consumed bool, invocationDone <-chan struct{}) bool {
	chunk.acknowledged <- consumed
	if consumed {
		return true
	}
	select {
	case <-invocationDone:
	case <-ctx.Done():
	case <-time.After(streamStopGrace):
	}
	return false
}

type streamCompletion struct {
	reply *methodResult[jsontext.Value]
	err   error
}

func (c streamCompletion) result(maximum int64) (jsontext.Value, error) {
	if c.err != nil {
		return nil, c.err
	}
	if c.reply == nil {
		return nil, ErrStreamUnconfirmed
	}
	if int64(len(c.reply.valueJSON)) > maximum {
		return nil, errors.New("plugin: stream final result exceeds response capacity")
	}
	return c.reply.valueJSON, nil
}

func (b *Binding) streamReceiver(call context.Context, chunks chan<- pendingChunk, limits CallbackLimits) (Callback, error) {
	method, err := chunkDeliveryMethod()
	if err != nil {
		return Callback{}, err
	}
	var mu sync.Mutex
	var received uint64
	return NewCallback(method, nil, func(ctx context.Context, command Command[deliveredChunk]) (bool, error) {
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
		chunk := pendingChunk{value: append(jsontext.Value(nil), command.Input.Value...), acknowledged: make(chan bool, 1)}
		select {
		case chunks <- chunk:
		case <-ctx.Done():
			return false, context.Cause(ctx)
		case <-call.Done():
			return false, context.Cause(call)
		}
		select {
		case accepted := <-chunk.acknowledged:
			received++
			return accepted, nil
		case <-ctx.Done():
			return false, context.Cause(ctx)
		case <-call.Done():
			return false, context.Cause(call)
		}
	})
}

// StreamWithMetadata retains the original MCP invocation while acknowledged
// callback delivery keeps production behind actual consumer demand.
func (b *Binding) StreamWithMetadata(ctx context.Context, callbacks *Callbacks, limits CallbackLimits, id CommandID, input framevalue.JSON, metadata sdk.ParamsBase) iter.Seq2[StreamValue[jsontext.Value, jsontext.Value], error] {
	frozen := scopemetadata.Map(metadata.Meta).Clone()
	_, metadataErr := json.Marshal(sdk.ParamsBase{Meta: sdk.Meta(frozen)})
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
		metadata := sdk.ParamsBase{Meta: sdk.Meta(frozen.Clone())}
		if err := b.method.ValidateInput(input); err != nil {
			fail(err)
			return
		}
		call, cancel := context.WithCancel(ctx)
		defer cancel()
		chunks := make(chan pendingChunk)
		receiver, err := b.streamReceiver(call, chunks, limits)
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
		if err := metadata.Set(StreamMetadataKey, ref); err != nil {
			fail(err)
			return
		}
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
				consumed := yield(StreamValue[jsontext.Value, jsontext.Value]{Chunk: new(chunk.value)}, nil)
				if !chunk.acknowledge(call, consumed, done) {
					return
				}
			case <-done:
				result, err := completion.result(limits.MaxResponseBytes)
				if err != nil {
					if refusal, known := errors.AsType[*framemcp.Refusal](err); known {
						yield(StreamValue[jsontext.Value, jsontext.Value]{}, refusal)
					} else {
						fail(err)
					}
				} else {
					yield(StreamValue[jsontext.Value, jsontext.Value]{Result: new(result)}, nil)
				}
				return
			case <-call.Done():
				fail(context.Cause(call))
				return
			}
		}
	}
}

func (m *StreamingMethod[P, C, R]) StreamWithMetadata(ctx context.Context, binding *Binding, callbacks *Callbacks, limits CallbackLimits, id CommandID, input P, metadata sdk.ParamsBase) iter.Seq2[StreamValue[C, R], error] {
	admitted, admissionErr := framevalue.EncodeJSON(input)
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
				err = json.Unmarshal(*raw.Chunk, value.Chunk, json.RejectUnknownMembers(true), framevalue.Numbers())
			} else {
				value.Result = new(R)
				err = json.Unmarshal(*raw.Result, value.Result, json.RejectUnknownMembers(true), framevalue.Numbers())
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
