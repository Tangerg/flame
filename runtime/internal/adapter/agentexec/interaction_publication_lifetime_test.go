package agentexec

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestExecutionWaitsForSlowDurableReceipts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "echo", Description: "Return text."}, func(context.Context, struct{}) (string, error) { return "echoed", nil })
		if err != nil {
			t.Fatal(err)
		}
		model := &observationScriptModel{responses: []*chat.Response{
			interactionToolResponse(chat.ToolCall{ID: "echo_call", Name: "echo", Arguments: `{}`}, 1, 1),
			interactionTextResponse("done"),
		}}
		executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
			ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
			ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
		})
		ref, err := executor.StageRoot(t.Context(), interactionTestStart())
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := executor.Release(context.Background(), ref); err != nil {
				t.Error(err)
			}
		}()
		events, err := executor.Observe(t.Context(), ref)
		if err != nil {
			t.Fatal(err)
		}
		if err := executor.BeginRoot(t.Context(), ref); err != nil {
			t.Fatal(err)
		}
		delayed, completed := 0, false
		for event := range events {
			if lookup, ok := event.Payload.(runs.ResultPublicationLookup); ok {
				time.Sleep(time.Minute)
				delayed++
				lookup.Complete(false, nil)
				continue
			}
			if commit, ok := event.Payload.(runs.ExecutionFactCommit); ok {
				switch commit.Fact().(type) {
				case runs.ModelCallCompleted, runs.ToolResultsCommitted:
					time.Sleep(time.Minute)
					delayed++
				}
				commit.Complete(nil)
				event.Payload = commit.Fact()
			}
			if end, ok := event.Payload.(runs.SegmentEnded); ok {
				if end.Reason != run.OutcomeCompleted {
					t.Fatalf("slow persistence changed terminal: %+v", end)
				}
				completed = true
			}
		}
		if delayed != 4 || !completed {
			t.Fatalf("delayed=%v completed=%v", delayed, completed)
		}
	})
}

func TestObservedFactSurvivesExecutionCancellationUntilRelease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		execution, cancelExecution := context.WithCancel(t.Context())
		session := &interactionSession{lifetime: newInteractionLifetime(execution)}
		defer session.lifetime.beginRelease()
		ctx, cancel := session.lifetime.publicationContext(execution)
		defer cancel()
		result := make(chan error, 1)
		go func() {
			result <- session.commitFact(ctx, runs.ExecutorMember{}, runs.ModelCallFailed{CallID: "observed"})
		}()
		event := <-session.lifetime.events
		cancelExecution()
		if err := session.commitFact(context.Background(), runs.ExecutorMember{}, runs.ModelCallStarted{CallID: "new"}); !errors.Is(err, context.Canceled) {
			t.Fatalf("admitted new work after cancellation: %v", err)
		}
		select {
		case <-session.lifetime.events:
			t.Fatal("canceled admission entered publication")
		default:
		}
		time.Sleep(time.Minute)
		select {
		case err := <-result:
			t.Fatalf("abandoned known fact before release: %v", err)
		default:
		}
		event.Payload.(runs.ExecutionFactCommit).Complete(nil)
		if err := <-result; err != nil {
			t.Fatal(err)
		}

		go func() {
			result <- session.commitFact(ctx, runs.ExecutorMember{}, runs.ModelCallFailed{CallID: "pending"})
		}()
		<-session.lifetime.events
		session.lifetime.beginRelease()
		if err := <-result; !errors.Is(err, context.Canceled) && !errors.Is(err, errInteractionReleased) {
			t.Fatalf("release did not end publication: %v", err)
		}
	})
}
