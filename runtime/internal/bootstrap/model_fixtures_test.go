package bootstrap

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/invocation"
)

type replyStub struct {
	reply string
}

func newReplyStub(reply string) *replyStub {
	return &replyStub{reply: reply}
}

func (r *replyStub) Call(_ context.Context, _ *chat.Request) (*chat.Response, error) {
	message := chat.NewAssistantMessage(chat.NewTextPart(r.reply))
	return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
}

func (r *replyStub) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return testsupport.StreamResponse(r.Call(ctx, req))
}

type lifecycleModel struct {
	calls              atomic.Int32
	firstCallStarted   chan struct{}
	releaseFirstCall   chan struct{}
	resumedCallStarted chan struct{}
	releaseResumedCall chan struct{}
	firstStartedOnce   sync.Once
	resumedStartedOnce sync.Once
}

func newLifecycleModel() *lifecycleModel {
	return &lifecycleModel{
		firstCallStarted:   make(chan struct{}),
		releaseFirstCall:   make(chan struct{}),
		resumedCallStarted: make(chan struct{}),
		releaseResumedCall: make(chan struct{}),
	}
}

func (l *lifecycleModel) Call(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
	switch l.calls.Add(1) {
	case 1:
		l.firstStartedOnce.Do(func() { close(l.firstCallStarted) })
		select {
		case <-l.releaseFirstCall:
		case <-ctx.Done():
			return nil, &invocation.Failure{Kind: invocation.FailureNotExecuted, Cause: ctx.Err()}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
			ID:        "call_question",
			Name:      "ask_user",
			Arguments: `{"questions":[{"question":"Continue the lifecycle check?"}]}`,
		}))
		return chat.NewResponse(&chat.Output{
			Message: &message, FinishReason: chat.FinishReasonToolCalls,
		}, &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}})
	default:
		l.resumedStartedOnce.Do(func() { close(l.resumedCallStarted) })
		select {
		case <-l.releaseResumedCall:
			message := chat.NewAssistantMessage(chat.NewTextPart("settled before cancellation"))
			return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}})
		case <-ctx.Done():
			return nil, &invocation.Failure{Kind: invocation.FailureNotExecuted, Cause: ctx.Err()}
		}
	}
}

func (l *lifecycleModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return testsupport.StreamResponse(l.Call(ctx, request))
}
