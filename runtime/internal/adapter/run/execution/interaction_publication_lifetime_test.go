package execution

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestExecutionWaitsForSlowDurableReceipts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "fixture_echo", Description: "Return text."}, func(context.Context, struct{}) (string, error) { return "echoed", nil })
		if err != nil {
			t.Fatal(err)
		}
		model := &observationScriptModel{responses: []*chat.Response{
			interactionToolResponse(chat.ToolCall{ID: "echo_call", Name: "fixture_echo", Arguments: `{}`}, 1, 1),
			interactionTextResponse("done"),
		}}
		executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
			ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "echo")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
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
		events, err := observeTestInteraction(t, executor, t.Context(), ref)
		if err != nil {
			t.Fatal(err)
		}
		if err := executor.BeginRoot(t.Context(), ref); err != nil {
			t.Fatal(err)
		}
		delayed, completed := 0, false
		for event := range events {

			if commit, ok := event.Payload.(runs.ExecutionFactCommit); ok {
				switch commit.Fact().(type) {
				case runs.ModelCallCompleted, runs.ToolResultsCommitted:
					time.Sleep(time.Minute)
					delayed++
				}
				if _, modelCompletion := commit.Fact().(runs.ModelCallCompleted); modelCompletion {
					session, err := executor.session(ref)
					if err != nil {
						t.Fatal(err)
					}
					tree, err := session.engine.InspectTree(t.Context(), session.processRootID())
					if err != nil {
						t.Fatal(err)
					}
					root, found := tree.Process(session.processRootID())
					if !found || root.Snapshot.Status().Terminal() {
						t.Fatal("Scope completed before Runtime accepted the model response")
					}
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
		if delayed != 3 || !completed {
			t.Fatalf("delayed=%v completed=%v", delayed, completed)
		}
	})
}

func TestObservedFactSurvivesExecutionCancellationUntilRelease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		execution, cancelExecution := context.WithCancel(t.Context())
		session := &interactionSession{lifetime: newInteractionLifetime(execution)}
		detach, attached := session.state.attachObserver(t.Context())
		if !attached {
			t.Fatal("observation was not admitted")
		}
		defer detach()
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

func TestObservationWithdrawalEndsUnacknowledgedFact(t *testing.T) {
	for _, withdrawal := range []string{"cancel", "stop consuming"} {
		t.Run(withdrawal, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				executor := newTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
					t.Error("withdrawing observation dispatched model work")
					return nil, errors.New("unexpected model call")
				}))
				ref, err := executor.StageRoot(t.Context(), interactionTestStart())
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := executor.Release(context.WithoutCancel(t.Context()), ref); err != nil {
						t.Error(err)
					}
				}()
				observation, cancel := context.WithCancel(t.Context())
				defer cancel()
				events, err := executor.Observe(observation, ref)
				if err != nil {
					t.Fatal(err)
				}
				session, _ := executor.session(ref)
				result := make(chan error, 1)
				go func() {
					result <- session.commitFact(t.Context(), runs.ExecutorMember{}, runs.ModelCallFailed{CallID: "unacknowledged"})
				}()
				synctest.Wait()
				select {
				case err := <-result:
					t.Fatalf("fact did not await its observer: %v", err)
				default:
				}
				if withdrawal == "cancel" {
					cancel()
				} else {
					events(func(event runs.ExecutorEvent) bool {
						if _, authoritative := event.Payload.(runs.ExecutionFactCommit); !authoritative {
							t.Fatalf("unexpected publication: %T", event.Payload)
						}
						return false
					})
				}
				if err := <-result; !errors.Is(err, context.Canceled) {
					t.Fatalf("observer withdrawal did not end the receipt wait: %v", err)
				}
				if err := session.commitFact(t.Context(), runs.ExecutorMember{}, runs.ModelCallFailed{CallID: "later"}); !errors.Is(err, errInteractionReleased) && !errors.Is(err, context.Canceled) {
					t.Fatalf("retired observation admitted another fact: %v", err)
				}
			})
		})
	}
}
