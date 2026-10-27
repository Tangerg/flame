package mcp

import (
	"context"
	"errors"
	"iter"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
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
	if err := callbacks.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	binding, err := Resolve(t.Context(), session, method.Descriptor().Name)
	if err != nil {
		t.Fatal(err)
	}
	return binding, callbacks
}

func TestBackendStreamRetainsOriginalInvocationAndConsumerBackpressure(t *testing.T) {
	method, err := NewStreamingMethod[callbackInput, int64, bool](MethodConfig{Name: "records.stream", Kind: portable.Mutation, Target: "call", Description: "Stream records."})
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
	method, err := NewStreamingMethod[callbackInput, int64, bool](MethodConfig{Name: "records.stream", Kind: portable.Mutation, Target: "call", Description: "Cancelable stream."})
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
	method, err := NewStreamingMethod[callbackInput, int64, bool](MethodConfig{Name: "records.stream", Kind: portable.Mutation, Target: "call", Description: "Fail after a stream chunk."})
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
