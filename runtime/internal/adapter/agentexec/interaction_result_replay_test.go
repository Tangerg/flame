package agentexec

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestExecutionTreeReplayUsesStoredReceiptBeforeProductMetadata(t *testing.T) {
	for _, lostAcknowledgment := range []bool{false, true} {
		t.Run(map[bool]string{false: "duplicate", true: "lost acknowledgment"}[lostAcknowledgment], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			trees := &replayExecutionTrees{testExecutionTrees: testTrees(t)}
			var executions, modelCalls atomic.Int32
			executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "store", Description: "Store a result."}, func(context.Context, struct{}) (string, error) { executions.Add(1); return "stored once", nil })
			if err != nil {
				t.Fatal(err)
			}
			model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				modelCalls.Add(1)
				if hasToolMessage(request.Messages) {
					return interactionTextResponse("done"), nil
				}
				return interactionToolResponse(chat.ToolCall{ID: "store_once", Name: "store", Arguments: `{}`}, 1, 1), nil
			})
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
				ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
				ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{}, ExecutionTrees: trees,
			})
			start := interactionTestStart()
			ref, err := executor.StageRoot(ctx, start)
			if err != nil {
				t.Fatal(err)
			}
			defer executor.Release(context.Background(), ref)
			sequence, err := executor.Observe(ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			finished := make(chan error, 1)
			var publications int
			go func() {
				var terminal bool
				for event := range sequence {
					commit, ok := event.Payload.(runs.ExecutionFactCommit)
					if !ok {
						continue
					}
					var commitErr error
					if tree, ok := commit.Fact().(runs.ExecutionTreeSettled); ok {
						commitErr = trees.SaveExecutionTree(ctx, tree.Update)
						for _, fact := range tree.Facts {
							if results, ok := fact.Payload.(runs.ToolResultsCommitted); ok {
								publications++
								trees.record(results.Publication)
								snapshot, err := agent.ParseTreeSnapshot(tree.Update.Head.Payload)
								if err != nil {
									commitErr = err
									break
								}
								// A replacement projection owner has no per-Tool metadata. Durable
								// receipts must suppress replay before that missing metadata is read.
								replacement := &interactionSession{start: start, executionTrees: trees, lifetime: newInteractionLifetime(ctx), state: interactionState{observerWasAttached: true}}
								trees.loseReceipt = lostAcknowledgment
								commitErr = replacement.commitTree(ctx, snapshot, tree.Update.PreviousWriter, tree.Update.PreviousDigest)
								replacement.lifetime.beginRelease()
							}
							if end, ok := fact.Payload.(runs.SegmentEnded); ok {
								terminal = end.Reason == run.OutcomeCompleted
							}
						}
					}
					if end, ok := commit.Fact().(runs.SegmentEnded); ok {
						terminal = end.Reason == run.OutcomeCompleted
					}
					commit.Complete(commitErr)
					if commitErr != nil {
						finished <- commitErr
						return
					}
				}
				if !terminal {
					finished <- errors.New("execution did not complete")
					return
				}
				finished <- nil
			}()
			if err := executor.BeginRoot(ctx, ref); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-finished:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if executions.Load() != 1 || modelCalls.Load() != 2 || publications != 1 {
				t.Fatalf("replay repeated work: tools=%d models=%d publications=%d", executions.Load(), modelCalls.Load(), publications)
			}
		})
	}
}

type replayExecutionTrees struct {
	*testExecutionTrees
	loseReceipt bool
}

func (s *replayExecutionTrees) SaveExecutionTree(ctx context.Context, update runs.ExecutionTreeUpdate) error {
	if err := s.testExecutionTrees.SaveExecutionTree(ctx, update); err != nil {
		return err
	}
	if s.loseReceipt {
		s.loseReceipt = false
		return errors.New("commit acknowledgment lost")
	}
	return nil
}
