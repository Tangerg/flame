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

func TestUsageOnlyResponsePersistsCompletedCallAndRejectedRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	model := &usageOnlyProviderModel{}
	host, api := openProtocolRuntime(t, model)
	defer func() {
		if err := host.Close(); err != nil {
			t.Errorf("close Runtime: %v", err)
		}
	}()
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "usage without content",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "answer"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "usage-only response")
	ended, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if ended.Status != protocol.RunStatusFinished || ended.Outcome == nil ||
		ended.Outcome.Type != protocol.OutcomeFailed || ended.Outcome.Error == nil ||
		ended.Outcome.Error.Type != protocol.ProblemProviderRejected {
		t.Fatalf("Run = %+v, want Scope's definite response rejection", ended)
	}
	if ended.Metrics.Steps != 1 || ended.Metrics.Usage == nil ||
		ended.Metrics.Usage.InputTokens != 7 || ended.Metrics.Usage.OutputTokens != 2 {
		t.Fatalf("Run lost completed model accounting: %+v", ended.Metrics)
	}
	calls, err := api.ListModelInvocations(ctx, protocol.ListModelInvocationsRequest{RunID: started.RunID})
	if err != nil || len(calls.Data) != 1 {
		t.Fatalf("model calls = %+v, %v", calls, err)
	}
	call := calls.Data[0]
	if call.State != protocol.ModelInvocationCompleted || call.Usage == nil ||
		call.Usage.InputTokens != 7 || call.Usage.OutputTokens != 2 || call.FirstOutputLatencyMillis != nil {
		t.Fatalf("completed model call = %+v", call)
	}
	items, err := api.ListItems(ctx, protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: started.RunID},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items.Data {
		if item.Type == protocol.ItemTypeAgentMessage || item.Type == protocol.ItemTypeReasoning {
			t.Fatalf("usage-only response invented content: %+v", item)
		}
	}
	if model.calls.Load() != 1 {
		t.Fatalf("model calls = %d, want no retry after Scope rejection", model.calls.Load())
	}
}

type usageOnlyProviderModel struct{ calls atomic.Int32 }

func (m *usageOnlyProviderModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	m.calls.Add(1)
	return chat.NewResponse(&chat.Output{FinishReason: chat.FinishReasonStop}, &chat.ResponseMetadata{
		Model: "test-model", Usage: &chat.Usage{InputTokens: 7, OutputTokens: 2},
	})
}

func (m *usageOnlyProviderModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return testsupport.StreamResponse(m.Call(ctx, request))
}

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
	for runID, state := range map[string]protocol.ModelInvocationState{failed.RunID: protocol.ModelInvocationFailed, followUp.RunID: protocol.ModelInvocationCompleted} {
		calls, err := api.ListModelInvocations(ctx, protocol.ListModelInvocationsRequest{RunID: runID})
		if err != nil || len(calls.Data) != 1 {
			t.Fatalf("model calls for %s = %+v, %v", runID, calls, err)
		}
		call := calls.Data[0]
		if call.State != state || call.RunID != runID || call.CallID == "" || call.SegmentID == "" || call.StartedAt.IsZero() || call.SettledAt.Before(call.StartedAt) {
			t.Fatalf("model call = %+v, want %s", call, state)
		}
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
