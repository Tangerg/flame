package mcp

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tangerg/frame/portable"
	framevalue "github.com/Tangerg/frame/value"

	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestDelayedHostStreamRetainsItsPublishedMethod(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	if err := host.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	method, err := NewStreamingMethod[callbackInput, int64, bool](portable.MethodConfig{Name: "test/stream", Kind: portable.Command, Target: "call", Description: "Stream the captured owner."})
	if err != nil {
		t.Fatal(err)
	}
	callback, err := NewStreamingCallback(method, func(context.Context, Command[callbackInput]) iter.Seq2[StreamValue[int64, bool], error] {
		return func(yield func(StreamValue[int64, bool], error) bool) {
			yield(StreamValue[int64, bool]{Result: new(true)}, nil)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := host.Open(t.Context(), session, callbackTestLimits, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	stream := StreamHost(t.Context(), ref, method, "original-stream", callbackInput{Value: 7})
	*method = StreamingMethod[callbackInput, int64, bool]{}
	completed := false
	for value, err := range stream {
		if err != nil || value.Result == nil || !*value.Result {
			t.Fatalf("replacement changed the captured host stream: %+v, %v", value, err)
		}
		completed = true
	}
	if !completed {
		t.Fatal("original host stream did not complete")
	}
}

func TestStreamingCallbackNeverPublishesAPrematureFinalResult(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	if err := host.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	method, err := NewStreamingMethod[callbackInput, int64, bool](portable.MethodConfig{Name: "test/stream", Kind: portable.Command, Target: "call", Description: "Stream the captured owner."})
	if err != nil {
		t.Fatal(err)
	}
	callback, err := NewStreamingCallback(method, func(context.Context, Command[callbackInput]) iter.Seq2[StreamValue[int64, bool], error] {
		return func(yield func(StreamValue[int64, bool], error) bool) {
			if !yield(StreamValue[int64, bool]{Chunk: new(int64(1))}, nil) {
				return
			}
			if !yield(StreamValue[int64, bool]{Result: new(true)}, nil) {
				return
			}
			yield(StreamValue[int64, bool]{}, errors.New("failure after tentative final"))
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := host.Open(t.Context(), session, callbackTestLimits, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	chunks, finals, failures := 0, 0, 0
	for item, err := range StreamHost(t.Context(), ref, method, "original-stream", callbackInput{Value: 1}) {
		if err != nil {
			if !errors.Is(err, ErrStreamUnconfirmed) {
				t.Fatal(err)
			}
			failures++
		} else if item.Result != nil {
			finals++
		} else if item.Chunk != nil {
			chunks++
		}
	}
	if chunks != 1 || finals != 0 || failures != 1 {
		t.Fatalf("premature final accepted: chunks=%d finals=%d failures=%d", chunks, finals, failures)
	}
}

func TestStreamingWriteStopsTheSourceWithoutAnEventQueue(t *testing.T) {
	method, err := NewStreamingMethod[callbackInput, int64, bool](portable.MethodConfig{Name: "test/stream", Kind: portable.Command, Target: "call", Description: "Stream with transport backpressure."})
	if err != nil {
		t.Fatal(err)
	}
	produced := 0
	callback, err := NewStreamingCallback(method, func(context.Context, Command[callbackInput]) iter.Seq2[StreamValue[int64, bool], error] {
		return func(yield func(StreamValue[int64, bool], error) bool) {
			for index := int64(0); index < 100; index++ {
				produced++
				if !yield(StreamValue[int64, bool]{Chunk: new(index)}, nil) {
					return
				}
			}
			yield(StreamValue[int64, bool]{Result: new(true)}, nil)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	wire := &invokeParams{Input: []byte(`{"value":1}`), CommandID: "bounded-stream"}
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	writer := &failedStreamWriter{ResponseRecorder: httptest.NewRecorder()}
	serveCallbackStream(writer, request, callback.method, wire, callbackTestLimits)
	if produced != 1 {
		t.Fatalf("failed first transport write prefetched %d chunks", produced)
	}
}

func TestStreamSchemaOwnsIdentityAndUnaryInvocationRefusal(t *testing.T) {
	method, err := NewStreamingMethod[callbackInput, int64, bool](portable.MethodConfig{Name: "test/stream", Kind: portable.Command, Target: "call", Description: "Stream the captured source."})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := method.Descriptor()
	stream, err := portable.AdmitMethod(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	descriptor.Stream = nil
	unary, err := portable.AdmitMethod(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if stream.Identity() == unary.Identity() {
		t.Fatal("streaming schema was omitted from call identity")
	}
	descriptor = stream.Descriptor()
	descriptor.Stream[0] = '!'
	if _, err := portable.AdmitMethod(descriptor); err == nil {
		t.Fatal("invalid stream schema admitted")
	}
	if err := stream.ValidateChunk([]byte(`1`)); err != nil {
		t.Fatal(err)
	}
	binding := &Binding{method: stream}
	if _, err := binding.Invoke(t.Context(), "original-stream", framevalue.JSON{}); err == nil {
		t.Fatal("stream entered unary delivery")
	}
}

type failedStreamWriter struct{ *httptest.ResponseRecorder }

func (*failedStreamWriter) Write([]byte) (int, error) { return 0, errors.New("stopped consumer") }
