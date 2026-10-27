package bootstrap

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	chathistory "github.com/Tangerg/scope/core/history"
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
		original, err = stores.ChatHistory.Read(ctx, chathistory.ConversationID(sessionID))
		if err != nil {
			return nil, err
		}
		response := completedTextResponse("## Goal\nKeep the original")
		response.Output.FinishReason = chat.FinishReasonLength
		return response, nil
	})}
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	host, _, storage := openProtocolRuntimeWithStores(t, model)
	stores = storage
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Errorf("close runtime: %v", err)
		}
	})
	ctx := t.Context()
	endpoint := host.Endpoint()
	options := delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}}
	created := endpoint.Invoke(ctx, delivery.SessionsCreate, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "truncated compaction summary",
	}, options)
	if created.Failure != nil {
		t.Fatal(created.Failure)
	}
	sessionID = created.Value.(*protocol.Session).ID
	startOptions := options
	startOptions.IdempotencyKey = "truncated-summary-start"
	started := endpoint.Invoke(ctx, delivery.RunsStart, protocol.StartRunRequest{
		SessionID: sessionID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Continue the long tool loop."}},
	}, startOptions)
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
	completionFailure := (&chatclient.OutputCompletionError{FinishReason: chat.FinishReasonLength}).Error()
	if outcome == nil || outcome.Type != protocol.OutcomeFailed || outcome.Error == nil ||
		!strings.Contains(outcome.Error.Detail, completionFailure) {
		t.Fatalf("Run outcome = %+v, want incomplete summary failure", outcome)
	}
	history, err := stores.ChatHistory.Read(ctx, chathistory.ConversationID(sessionID))
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
