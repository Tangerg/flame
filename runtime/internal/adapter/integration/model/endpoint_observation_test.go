package model

import (
	"context"
	"iter"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	modelchoice "github.com/Tangerg/flame/runtime/model"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
)

type endpointStreamFixture struct{}

func (endpointStreamFixture) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return nil, context.Canceled
}
func (endpointStreamFixture) Stream(context.Context, *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return func(yield func(*chat.ResponseDelta, error) bool) {
		if !yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("known ")}}, nil) {
			return
		}
		yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("stream")}, FinishReason: chat.FinishReasonStop, Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: 7, OutputTokens: 2}}}, nil)
	}
}

func TestEndpointStreamRecordsCompletionAndConsumerStopWithoutPromotion(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewModelInvocationStore(db)
	resolve, err := NewObservedResolver(recordingChatResolver{resolve: func(_ modelchoice.Selection) (ResolvedChat, error) {
		return NewResolvedChat(endpointStreamFixture{}, nil)
	}}, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolve.ResolveChat(t.Context(), testsupport.MustModelSelection("anthropic", "captured-model"))
	if err != nil {
		t.Fatal(err)
	}
	stream, _ := resolved.Streamer()
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original input")))
	for _, id := range []string{"complete", "stop", "stop-final"} {
		ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: id, Origin: capability.Automatic, Parent: "original-operation", Target: capability.Target{Scope: "call", Reference: id}})
		if err != nil {
			t.Fatal(err)
		}
		sequence := stream.Stream(ctx, request)
		if rows, err := store.PageEndpointInvocations(t.Context(), id, 0, "", 10); err != nil || len(rows) != 0 {
			t.Fatal("unconsumed iterator published endpoint work")
		}
		for delta, err := range sequence {
			if err != nil {
				t.Fatal(err)
			}
			if id == "stop" {
				break
			}
			if id == "stop-final" && delta.FinishReason != "" {
				break
			}
		}
		rows, err := store.PageEndpointInvocations(t.Context(), id, 0, "", 10)
		if err != nil || len(rows) != 1 {
			t.Fatalf("endpoint observation: %+v, %v", rows, err)
		}
		if id == "complete" && (rows[0].State != "completed" || rows[0].Response == nil || rows[0].Response.Text() != "known stream" || rows[0].Response.Metadata.Usage.InputTokens != 7) {
			t.Fatal("completion lost exact response/usage")
		}
		if id == "stop" && (rows[0].State != "unknown" || rows[0].Response != nil || rows[0].Failure == "") {
			t.Fatal("consumer stop became completed provider call")
		}
		if id == "stop-final" && (rows[0].State != "failed" || rows[0].Response == nil || rows[0].Response.Text() != "known stream" || rows[0].Failure == "") {
			t.Fatal("consumer stop discarded complete known response or promoted operation success")
		}
	}
}
