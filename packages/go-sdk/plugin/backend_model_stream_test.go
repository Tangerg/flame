package plugin

import (
	"context"
	"errors"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"iter"
	"sync/atomic"
	"testing"
)

func TestBackendDecoratorStreamsThroughCapturedModelNextWithoutDeadlock(t *testing.T) {
	method, err := ModelStreamMethod()
	if err != nil {
		t.Fatal(err)
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "external-model-decorator"}, nil)
	backend, err := qimumcp.NewBackend(server)
	if err != nil {
		t.Fatal(err)
	}
	if err := method.RegisterServer(backend, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[chat.Request]) iter.Seq2[qimumcp.StreamValue[chat.ResponseDelta, ModelResult], error] {
		return func(yield func(qimumcp.StreamValue[chat.ResponseDelta, ModelResult], error) bool) {
			next, err := qimumcp.CallbackFromContext(ctx)
			if err != nil {
				yield(qimumcp.StreamValue[chat.ResponseDelta, ModelResult]{}, err)
				return
			}
			request := command.Input.Clone()
			request.Messages[0] = chat.NewUserMessage(chat.NewTextPart("transformed by decorator"))
			if !yield(qimumcp.StreamValue[chat.ResponseDelta, ModelResult]{Chunk: &chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("outer ")}}}, nil) {
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
	if err := qimumcp.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, server, client)
	callbacks := newTestCallbacks(t)
	if err := callbacks.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	var innerCalls atomic.Int32
	next, err := NewModelStreamCallback(func(_ context.Context, command qimumcp.Command[chat.Request]) iter.Seq2[*chat.ResponseDelta, error] {
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
	ref, release, err := callbacks.Open(t.Context(), session, qimumcp.CallbackLimits{MaxRequestBytes: 64 * 1024, MaxResponseBytes: 64 * 1024}, next)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	metadata, err := qimumcp.CallbackMetadata(ref)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := qimumcp.Resolve(t.Context(), session, method.Descriptor().Name)
	if err != nil {
		t.Fatal(err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original")))
	if err != nil {
		t.Fatal(err)
	}
	var outer chat.ResponseAccumulator
	var inner *ModelResult
	sequence := method.StreamWithMetadata(t.Context(), binding, callbacks, qimumcp.CallbackLimits{MaxRequestBytes: 64 * 1024, MaxResponseBytes: 64 * 1024}, "original-decorated-model", *request, metadata)
	for index := range metadata.Meta[qimumcp.CallbackMetadataKey] {
		metadata.Meta[qimumcp.CallbackMetadataKey][index] = ' '
	}
	if err := metadata.Set(qimumcp.CallbackMetadataKey, "replacement callback"); err != nil {
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
