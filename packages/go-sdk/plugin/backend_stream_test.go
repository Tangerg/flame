package plugin

import (
	"context"
	"errors"
	"iter"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func backendStreamingSession(t *testing.T, method *StreamingMethod[callbackInput, int64, bool], handler func(context.Context, *sdk.ServerSession, Command[callbackInput]) iter.Seq2[StreamValue[int64, bool], error]) (*Binding, *Callbacks) {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "streaming-owner"}, nil)
	backend, err := NewBackend(server)
	if err != nil {
		t.Fatal(err)
	}
	if err := method.RegisterServer(backend, handler); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "streaming-caller"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	callbacks := newTestCallbacks(t)
	callbacks.AdmitSession(session)
	binding, err := Resolve(t.Context(), session, method.Descriptor().Name)
	if err != nil {
		t.Fatal(err)
	}
	return binding, callbacks
}

func TestBackendDecoratorStreamsThroughCapturedModelNextWithoutDeadlock(t *testing.T) {
	method, err := ModelStreamMethod()
	if err != nil {
		t.Fatal(err)
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "external-model-decorator"}, nil)
	backend, err := NewBackend(server)
	if err != nil {
		t.Fatal(err)
	}
	if err := method.RegisterServer(backend, func(ctx context.Context, _ *sdk.ServerSession, command Command[chat.Request]) iter.Seq2[StreamValue[chat.ResponseDelta, ModelResult], error] {
		return func(yield func(StreamValue[chat.ResponseDelta, ModelResult], error) bool) {
			next, err := CallbackFromContext(ctx)
			if err != nil {
				yield(StreamValue[chat.ResponseDelta, ModelResult]{}, err)
				return
			}
			request := command.Input.Clone()
			request.Messages[0] = chat.NewUserMessage(chat.NewTextPart("transformed by decorator"))
			if !yield(StreamValue[chat.ResponseDelta, ModelResult]{Chunk: &chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("outer ")}}}, nil) {
				return
			}
			for value, err := range StreamModel(ctx, next, command.ID, request) {
				if !yield(value, err) {
					return
				}
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "host-model-chain"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	callbacks := newTestCallbacks(t)
	callbacks.AdmitSession(session)
	var innerCalls atomic.Int32
	next, err := NewModelStreamCallback(func(_ context.Context, command Command[chat.Request]) iter.Seq2[*chat.ResponseDelta, error] {
		return func(yield func(*chat.ResponseDelta, error) bool) {
			innerCalls.Add(1)
			if command.ID != "original-decorated-model" || command.Input.Messages[0].Text() != "transformed by decorator" {
				yield(nil, errors.New("original model next was replaced"))
				return
			}
			yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("inner")}, FinishReason: chat.FinishReasonStop, Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: 11, OutputTokens: 2}}}, nil)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := callbacks.Open(t.Context(), session, CallbackLimits{MaxRequestBytes: 64 * 1024, MaxResponseBytes: 64 * 1024}, next)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	metadata, err := CallbackMetadata(ref)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := Resolve(t.Context(), session, method.Descriptor().Name)
	if err != nil {
		t.Fatal(err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original")))
	if err != nil {
		t.Fatal(err)
	}
	var outer chat.ResponseAccumulator
	var inner *ModelResult
	sequence := method.StreamWithMetadata(t.Context(), binding, callbacks, CallbackLimits{MaxRequestBytes: 64 * 1024, MaxResponseBytes: 64 * 1024}, "original-decorated-model", *request, metadata)
	for index := range metadata.Meta[callbackMetadataKey] {
		metadata.Meta[callbackMetadataKey][index] = ' '
	}
	if err := metadata.Set(callbackMetadataKey, "replacement callback"); err != nil {
		t.Fatal(err)
	}
	for value, err := range sequence {
		if err != nil {
			t.Fatal(err)
		}
		if value.Chunk != nil {
			if err := outer.Add(value.Chunk); err != nil {
				t.Fatal(err)
			}
		} else {
			inner = value.Result
		}
	}
	response, err := outer.Response()
	if err != nil {
		t.Fatal(err)
	}
	if response.Text() != "outer inner" || inner == nil || inner.Response.Text() != "inner" || inner.Response.Metadata.Usage.InputTokens != 11 || innerCalls.Load() != 1 {
		t.Fatalf("duplex decorator lost evidence: outer=%+v inner=%+v calls=%d", response, inner, innerCalls.Load())
	}
}

func TestBackendStreamRetainsOriginalInvocationAndConsumerBackpressure(t *testing.T) {
	method, err := NewStreamingMethod[callbackInput, int64, bool](MethodConfig{Name: "records.stream", Kind: extension.Mutation, Target: "call", Description: "Stream records."})
	if err != nil {
		t.Fatal(err)
	}
	var produced atomic.Int32
	binding, callbacks := backendStreamingSession(t, method, func(_ context.Context, _ *sdk.ServerSession, command Command[callbackInput]) iter.Seq2[StreamValue[int64, bool], error] {
		return func(yield func(StreamValue[int64, bool], error) bool) {
			if command.ID != "original-stream" || command.Input.Value != 7 {
				yield(StreamValue[int64, bool]{}, errors.New("original invocation changed"))
				return
			}
			for index := int64(1); index <= 3; index++ {
				produced.Add(1)
				if !yield(StreamValue[int64, bool]{Chunk: new(index)}, nil) {
					return
				}
			}
			yield(StreamValue[int64, bool]{Result: new(true)}, nil)
		}
	})
	stream := method.StreamWithMetadata(t.Context(), binding, callbacks, callbackTestLimits, "original-stream", callbackInput{Value: 7}, sdk.ParamsBase{})
	if produced.Load() != 0 {
		t.Fatal("unconsumed stream started backend I/O")
	}
	var chunks int64
	final := false
	for value, err := range stream {
		if err != nil {
			t.Fatal(err)
		}
		if value.Chunk != nil {
			chunks++
			if *value.Chunk != chunks || produced.Load() != int32(chunks) {
				t.Fatalf("producer outran consumption: chunk=%d produced=%d", chunks, produced.Load())
			}
		} else {
			final = value.Result != nil && *value.Result
		}
	}
	if chunks != 3 || !final {
		t.Fatalf("stream outcome=%d %v", chunks, final)
	}
}

func TestStoppingBackendStreamCancelsProducerWithoutAnotherInvocation(t *testing.T) {
	method, err := NewStreamingMethod[callbackInput, int64, bool](MethodConfig{Name: "records.stream", Kind: extension.Mutation, Target: "call", Description: "Cancelable stream."})
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	var produced atomic.Int32
	binding, callbacks := backendStreamingSession(t, method, func(ctx context.Context, _ *sdk.ServerSession, _ Command[callbackInput]) iter.Seq2[StreamValue[int64, bool], error] {
		return func(yield func(StreamValue[int64, bool], error) bool) {
			defer close(stopped)
			produced.Add(1)
			if !yield(StreamValue[int64, bool]{Chunk: new(int64(1))}, nil) {
				return
			}
			<-ctx.Done()
		}
	})
	for value, err := range method.StreamWithMetadata(t.Context(), binding, callbacks, callbackTestLimits, "stopped-stream", callbackInput{Value: 7}, sdk.ParamsBase{}) {
		if err != nil || value.Chunk == nil {
			t.Fatalf("partial stream=%+v %v", value, err)
		}
		break
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("stopped consumer retained backend work")
	}
	if produced.Load() != 1 {
		t.Fatalf("stopped stream repeated producer work: %d", produced.Load())
	}
}

func TestBackendStreamFailureKeepsPartialEvidenceWithoutRetry(t *testing.T) {
	method, err := NewStreamingMethod[callbackInput, int64, bool](MethodConfig{Name: "records.stream", Kind: extension.Mutation, Target: "call", Description: "Fail after a stream chunk."})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	binding, callbacks := backendStreamingSession(t, method, func(context.Context, *sdk.ServerSession, Command[callbackInput]) iter.Seq2[StreamValue[int64, bool], error] {
		return func(yield func(StreamValue[int64, bool], error) bool) {
			calls.Add(1)
			if !yield(StreamValue[int64, bool]{Chunk: new(int64(1))}, nil) {
				return
			}
			yield(StreamValue[int64, bool]{}, errors.New("unknown after output"))
		}
	})
	chunks, finals, failures := 0, 0, 0
	for value, err := range method.StreamWithMetadata(t.Context(), binding, callbacks, callbackTestLimits, "unknown-stream", callbackInput{Value: 7}, sdk.ParamsBase{}) {
		if err != nil {
			if !errors.Is(err, ErrStreamUnconfirmed) {
				t.Fatal(err)
			}
			failures++
		} else if value.Chunk != nil {
			chunks++
		} else {
			finals++
		}
	}
	if chunks != 1 || finals != 0 || failures != 1 || calls.Load() != 1 {
		t.Fatalf("failed stream was replayed or completed: chunks=%d finals=%d errors=%d calls=%d", chunks, finals, failures, calls.Load())
	}
}
