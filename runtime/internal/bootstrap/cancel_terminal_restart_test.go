package bootstrap

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/scope/core/chat"
)

func TestCancelAndNaturalCompletionPreserveTerminalAcrossRestart(t *testing.T) {
	for _, naturalWins := range []bool{true, false} {
		name := "cancel wins"
		if naturalWins {
			name = "natural completion wins"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("FLAME_HOME", home)
			stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
			if err != nil {
				t.Fatal(err)
			}
			entered, finish := make(chan struct{}), make(chan struct{})
			model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
				if len(request.Tools) == 0 {
					return newReplyStub("terminal restart title").Call(ctx, request)
				}
				close(entered)
				select {
				case <-finish:
					return newReplyStub("naturally completed").Call(ctx, request)
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			})}
			host, api := buildProtocolRuntime(t, protocolRuntimeConfig(t, stores, model), home)
			t.Cleanup(func() { _ = host.Close() })
			ctx := protocolLifecycleContext(t.Context())
			session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}})
			if err != nil {
				t.Fatal(err)
			}
			started, stream, err := api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "finish this run"}}})
			if err != nil {
				t.Fatal(err)
			}
			events := collectRunEvents(stream)
			select {
			case <-entered:
			case <-time.After(lifecycleWaitBudget(t)):
				t.Fatal("model did not begin")
			}
			endpoint := host.Endpoint()
			options := acceptedCommandOptions()
			options.IdempotencyKey = "cancel-original"
			options.RequestMeta.ClientCapabilities.InterruptTypes = []protocol.InterruptType{protocol.InterruptQuestion}
			type result struct {
				response *protocol.CancelRunResponse
				err      error
			}
			cancelReady, deliverCancel, canceled := make(chan struct{}), make(chan struct{}), make(chan result, 1)
			go func() {
				close(cancelReady)
				select {
				case <-deliverCancel:
				case <-t.Context().Done():
					return
				}
				response, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](ctx, delivery.RunsCancel, protocol.CancelRunRequest{RunID: started.RunID, Reason: "operator stopped"}, options)
				canceled <- result{response, err}
			}()
			<-cancelReady
			if naturalWins {
				close(finish)
				waitForRunEvents(t, events, "natural terminal")
			}
			close(deliverCancel)
			var cancellation result
			select {
			case cancellation = <-canceled:
			case <-time.After(lifecycleWaitBudget(t)):
				t.Fatal("cancel did not settle")
			}
			outcome := protocol.OutcomeCanceled
			if naturalWins {
				outcome = protocol.OutcomeCompleted
				if cancellation.response != nil || !errors.Is(cancellation.err, protocol.ErrRunFinished) {
					t.Fatalf("natural winner became cancellation: %+v,%v", cancellation.response, cancellation.err)
				}
			} else {
				if cancellation.err != nil || cancellation.response == nil || cancellation.response.Run.Outcome.Type != outcome {
					t.Fatalf("cancel winner=%+v,%v", cancellation.response, cancellation.err)
				}
				waitForRunEvents(t, events, "canceled terminal")
			}
			before, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
			if err != nil || before.Status != protocol.RunStatusFinished || before.Outcome.Type != outcome {
				t.Fatalf("terminal=%+v,%v", before, err)
			}
			if naturalWins && before.Outcome.Detail != "" {
				t.Fatalf("cancel reason contaminated natural completion: %+v", before.Outcome)
			}
			if !naturalWins && before.Outcome.Detail != "operator stopped" {
				t.Fatalf("cancel reason lost: %+v", before.Outcome)
			}
			if err := host.Close(); err != nil {
				t.Fatal(err)
			}
			restarted, cold := openProtocolRuntime(t, newReplyStub("must not run"))
			t.Cleanup(func() { _ = restarted.Close() })
			snapshot, err := cold.GetSessionSnapshot(ctx, protocol.GetSessionSnapshotRequest{SessionID: session.ID})
			if err != nil || len(snapshot.Runs) != 1 || !reflect.DeepEqual(snapshot.Runs[0], *before) {
				t.Fatalf("cold terminal changed: %+v,%v; original=%+v", snapshot, err, before)
			}
		})
	}
}
