package bootstrap

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

func TestRuntimeRejectsTruncatedSummaryBeforeReplacingHistory(t *testing.T) {
	longRun := &longContextModel{}
	var stores *persistence.Bundle
	var sessionID string
	var original []chat.Message
	model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		if !isCompactionRequest(request) {
			return longRun.Call(ctx, request)
		}
		var err error
		original, err = stores.ChatHistory.Read(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		response := completedTextResponse("## Goal\nKeep the original")
		response.Output.FinishReason = chat.FinishReasonLength
		return response, nil
	})}
	storage, handler, ctx, home := newSessionStateE2ERuntime(t, model)
	stores = storage
	endpoint, err := delivery.NewEndpoint(handler, delivery.EndpointConfig{Lifetime: ctx})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		endpoint.BeginShutdown()
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := endpoint.AwaitShutdown(cleanup); err != nil {
			t.Errorf("close endpoint: %v", err)
		}
	})
	options := delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}}
	created := endpoint.Invoke(ctx, delivery.SessionsCreate, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "truncated compaction summary",
	}, options)
	if created.Failure != nil {
		t.Fatal(created.Failure)
	}
	sessionID = created.Value.(*protocol.Session).ID
	started := endpoint.Invoke(ctx, delivery.RunsStart, protocol.StartRunRequest{
		SessionID: sessionID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Continue the long tool loop."}},
	}, options)
	if started.Failure != nil {
		t.Fatal(started.Failure)
	}
	for _, err := range started.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	finished := endpoint.Invoke(ctx, delivery.RunsGet, protocol.GetRunRequest{
		RunID: started.Value.(*protocol.StartRunResponse).RunID,
	}, options)
	if finished.Failure != nil {
		t.Fatal(finished.Failure)
	}
	outcome := finished.Value.(*protocol.RunRef).Outcome
	if outcome == nil || outcome.Type != protocol.OutcomeFailed || outcome.Error == nil ||
		!strings.Contains(outcome.Error.Detail, `finish reason "length"`) {
		t.Fatalf("Run outcome = %+v, want incomplete summary failure", outcome)
	}
	history, err := stores.ChatHistory.Read(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(original) == 0 || !reflect.DeepEqual(history, original) {
		t.Fatal("failed compaction did not preserve the original model history")
	}
	mainCalls, _, compactedCalls, _ := longRun.Snapshot()
	if mainCalls > modelCallsBeforeMidRunCompaction || compactedCalls != 0 {
		t.Fatalf("model calls = %d/%d compacted, want no continuation from a truncated summary", mainCalls, compactedCalls)
	}
}
