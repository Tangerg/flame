package bootstrap

import (
	"context"
	"iter"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

func TestProviderFailureTerminalizesRunAndReleasesSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	model := &providerFailureThenReplyModel{}
	host, api := openProtocolRuntime(t, model)
	defer func() {
		if err := host.Close(); err != nil {
			t.Errorf("close Runtime: %v", err)
		}
	}()
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "provider failure recovery",
	})
	if err != nil {
		t.Fatal(err)
	}

	failed, failedEvents, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "fail once"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(failedEvents), "provider-failed Run")
	failedRun, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: failed.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if failedRun.Status != protocol.RunStatusFinished || failedRun.Outcome == nil ||
		failedRun.Outcome.Type != protocol.OutcomeFailed || failedRun.Outcome.Error == nil ||
		failedRun.Outcome.Error.Type != protocol.ProblemRateLimited ||
		failedRun.Outcome.Error.RetryAfterSeconds != 1 {
		t.Fatalf("provider-failed Run = %+v, want rate_limited with 1s minimum retry", failedRun)
	}

	followUp, followUpEvents, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "continue"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(followUpEvents), "post-failure Run")
	completed, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: followUp.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != protocol.RunStatusFinished || completed.Outcome == nil ||
		completed.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("post-failure Run = %+v, want completed", completed)
	}
}

type providerFailureThenReplyModel struct {
	calls atomic.Int32
}

func (p *providerFailureThenReplyModel) Call(
	context.Context,
	*chat.Request,
) (*chat.Response, error) {
	if p.calls.Add(1) == 1 {
		return nil, &run.FailureError{
			Kind:       run.FailureRateLimited,
			RetryAfter: 250 * time.Millisecond,
		}
	}
	message := chat.NewAssistantMessage(chat.NewTextPart("recovered"))
	return chat.NewResponse(&chat.Output{
		Message: &message, FinishReason: chat.FinishReasonStop,
	}, nil)
}

func (p *providerFailureThenReplyModel) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.ResponseDelta, error] {
	return testsupport.StreamResponse(p.Call(ctx, request))
}
