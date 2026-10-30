package model

import (
	"context"
	"errors"
	"iter"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/catalog"
)

func TestEndpointRecordsOneFailureForInvalidResponse(t *testing.T) {
	db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "flame.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := db.ModelInvocations
	cause := errors.New("provider returned malformed response")
	invalid := &chat.Response{}
	model := &observedEndpoint{
		inner: chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
			return invalid, cause
		}),
		journal: store, selection: testsupport.MustModelSelection("anthropic", "captured-model"),
	}
	ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: "invalid-response", Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeCall, Reference: "invalid-response"}})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original input")))
	response, err := model.Call(ctx, request)
	if response != invalid || !errors.Is(err, cause) || !errors.Is(err, chat.ErrInvalidResponse) {
		t.Fatalf("invalid response evidence changed: %+v, %v", response, err)
	}
	rows, err := store.PageEndpointInvocations(t.Context(), "invalid-response", 0, "", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("endpoint observation: %+v, %v", rows, err)
	}
	want := errors.Join(cause, invalid.Validate()).Error()
	if rows[0].Response != nil || rows[0].State != "unknown" || rows[0].Failure != want {
		t.Fatalf("invalid response was promoted or its failure was duplicated: %+v; want failure %q", rows[0], want)
	}
}

func TestEndpointRetainsGenerationWhenCatalogCannotPriceUsage(t *testing.T) {
	db, err := persistencefixture.Open(t, t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	message := chat.NewAssistantMessage(chat.NewTextPart("generated answer"))
	response := &chat.Response{
		Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop},
		Metadata: &chat.ResponseMetadata{Model: "claude-opus-5", Usage: &chat.Usage{
			InputTokens: 10, CacheReadInputTokens: new(int64(6)), CacheWriteInputTokens: new(int64(6)),
		}},
	}
	calls := 0
	endpoint := &observedEndpoint{
		inner: chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
			calls++
			return response, nil
		}),
		journal: db.ModelInvocations, selection: testsupport.MustModelSelection("anthropic", "claude-opus-5"), pricing: Pricing("anthropic", catalog.Default.Models("anthropic")),
	}
	ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: "pricing-failure", Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeCall, Reference: "pricing-failure"}})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("input")))
	got, callErr := endpoint.Call(ctx, request)
	if callErr == nil || got != response || calls != 1 {
		t.Fatalf("pricing failure lost generation or repeated inference: %+v, %v, calls=%d", got, callErr, calls)
	}
	rows, err := db.ModelInvocations.PageEndpointInvocations(t.Context(), "pricing-failure", 0, "", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("endpoint observation: %+v, %v", rows, err)
	}
	row := rows[0]
	if row.State != "failed" || row.Response == nil || row.Response.Text() != "generated answer" || row.Usage == nil || row.Usage.InputTokens != 10 || row.CostUSD != nil || row.Failure != callErr.Error() {
		t.Fatalf("pricing failure lost physical evidence or invented a price: %+v", row)
	}
}

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
	db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "flame.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := db.ModelInvocations
	resolve, err := NewObservedResolver(recordingChatResolver{resolve: func(_ modelchoice.Selection) (ResolvedChat, error) {
		return NewResolvedChat(endpointStreamFixture{}, nil, nil)
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
		ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: id, Origin: extension.Automatic, Parent: "original-operation", Target: portable.Target{Scope: extension.ScopeCall, Reference: id}})
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
