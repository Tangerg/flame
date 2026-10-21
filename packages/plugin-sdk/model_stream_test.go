package plugin

import (
	"context"
	"errors"
	"iter"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func TestCapturedModelStreamIsLazyAndUsesOneCanonicalAggregate(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	host.AdmitSession(session)
	var calls atomic.Int32
	callback, err := NewModelStreamCallback(func(_ context.Context, command Command[chat.Request]) iter.Seq2[*chat.ResponseDelta, error] {
		return func(yield func(*chat.ResponseDelta, error) bool) {
			calls.Add(1)
			if command.ID != "original-stream" || command.Input.Messages[0].Text() != "original request" {
				yield(nil, errors.New("stream capture changed"))
				return
			}
			if !yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("first ")}, Metadata: &chat.ResponseMetadata{Model: "inner-model", Usage: &chat.Usage{InputTokens: 9, OutputTokens: 1}}}, nil) {
				return
			}
			yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("second")}, FinishReason: chat.FinishReasonStop, Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: 9, OutputTokens: 2, CacheReadInputTokens: new(int64(0))}}}, nil)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := host.Open(t.Context(), session, CallbackLimits{MaxRequestBytes: 64 * 1024, MaxResponseBytes: 64 * 1024}, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original request")))
	if err != nil {
		t.Fatal(err)
	}
	sequence := StreamModel(t.Context(), ref, "original-stream", request)
	request.Messages[0] = chat.NewUserMessage(chat.NewTextPart("changed request"))
	if calls.Load() != 0 {
		t.Fatal("unconsumed stream started model work")
	}
	var accumulated chat.ResponseAccumulator
	var final *ModelResult
	for item, err := range sequence {
		if err != nil {
			t.Fatal(err)
		}
		if item.Chunk != nil {
			if final != nil {
				t.Fatal("chunk followed final response")
			}
			if err := accumulated.Add(item.Chunk); err != nil {
				t.Fatal(err)
			}
		} else {
			final = item.Result
		}
	}
	response, err := accumulated.Response()
	if err != nil {
		t.Fatal(err)
	}
	if final == nil || final.Failure != nil || final.Response.Text() != response.Text() || response.Text() != "first second" || final.Response.Metadata.Model != "inner-model" || final.Response.Metadata.Usage.OutputTokens != 2 || final.Response.Metadata.Usage.CacheReadInputTokens == nil || calls.Load() != 1 {
		t.Fatalf("canonical aggregate lost: %+v", final)
	}
}

func TestStoppingModelStreamCancelsAndJoinsItsSource(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	host.AdmitSession(session)
	stopped := make(chan struct{})
	callback, err := NewModelStreamCallback(func(ctx context.Context, _ Command[chat.Request]) iter.Seq2[*chat.ResponseDelta, error] {
		return func(yield func(*chat.ResponseDelta, error) bool) {
			defer close(stopped)
			if !yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("partial")}}, nil) {
				return
			}
			<-ctx.Done()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := host.Open(t.Context(), session, CallbackLimits{MaxRequestBytes: 64 * 1024, MaxResponseBytes: 64 * 1024}, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if err != nil {
		t.Fatal(err)
	}
	for item, err := range StreamModel(t.Context(), ref, "stopped-stream", request) {
		if err != nil || item.Chunk == nil {
			t.Fatalf("missing partial evidence: %+v %v", item, err)
		}
		break
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("stopped stream retained provider work")
	}
}

func TestModelStreamRetainsCompletedInnerResponseAfterOuterFailure(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	host.AdmitSession(session)
	callback, err := NewModelStreamCallback(func(context.Context, Command[chat.Request]) iter.Seq2[*chat.ResponseDelta, error] {
		return func(yield func(*chat.ResponseDelta, error) bool) {
			if !yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("known result")}, FinishReason: chat.FinishReasonStop, Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: 9, OutputTokens: 2}}}, nil) {
				return
			}
			yield(nil, context.DeadlineExceeded)
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
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if err != nil {
		t.Fatal(err)
	}
	var final *ModelResult
	for item, err := range StreamModel(t.Context(), ref, "known-inner-stream", request) {
		if err != nil {
			t.Fatal(err)
		}
		if item.Result != nil {
			final = item.Result
		}
	}
	if final == nil || final.Response == nil || final.Response.Text() != "known result" || final.Response.Metadata.Usage.InputTokens != 9 || !errors.Is(final.Failure, context.DeadlineExceeded) {
		t.Fatalf("inner evidence lost: %+v", final)
	}
}

func TestModelStreamAggregateUsesTheCapturedByteCapacity(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	host.AdmitSession(session)
	produced := 0
	callback, err := NewModelStreamCallback(func(context.Context, Command[chat.Request]) iter.Seq2[*chat.ResponseDelta, error] {
		return func(yield func(*chat.ResponseDelta, error) bool) {
			for range 100 {
				produced++
				if !yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta(strings.Repeat("x", 64))}}, nil) {
					return
				}
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := host.Open(t.Context(), session, CallbackLimits{MaxRequestBytes: 4096, MaxResponseBytes: 512}, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if err != nil {
		t.Fatal(err)
	}
	var final *ModelResult
	for item, err := range StreamModel(t.Context(), ref, "bounded-model-stream", request) {
		if err != nil {
			t.Fatal(err)
		}
		if item.Result != nil {
			final = item.Result
		}
	}
	if final == nil || final.Failure == nil || final.Response != nil || produced >= 100 {
		t.Fatalf("aggregate ignored byte capacity: produced=%d outcome=%+v", produced, final)
	}
}
