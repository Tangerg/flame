package agentexec

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestResultCommitterReconcilesStoredReceiptBeforeProductMetadata(t *testing.T) {
	for _, replay := range []string{"duplicate", "lost acknowledgment", "new projection host"} {
		t.Run(replay, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			db, err := sqlite.Open(ctx, ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			state, history := sqlite.NewRunStore(db), sqlite.NewMessageStore(db)
			start := interactionTestStart()
			draft := run.Draft{RunID: "run_replay", SessionID: start.SessionID, SegmentID: "segment_replay", CreatedAt: time.Now().UTC(), ModelSelection: start.ModelSelection}
			if err := state.Admit(ctx, draft); err != nil {
				t.Fatal(err)
			}
			var executions, modelCalls, publications atomic.Int32
			executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "store", Description: "Store a result."}, func(context.Context, struct{}) (string, error) {
				executions.Add(1)
				return "stored once", nil
			})
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
				ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
			})
			ref, err := executor.StageRoot(ctx, start)
			if err != nil {
				t.Fatal(err)
			}
			defer executor.Release(context.Background(), ref)
			session, err := executor.session(ref)
			if err != nil {
				t.Fatal(err)
			}
			handle := func(event runs.ExecutorEvent) {
				switch request := event.Payload.(type) {
				case runs.ResultPublicationLookup:
					publication := request.Publication()
					found, err := state.ResultPublicationCommitted(ctx, draft.SessionID, draft.RunID, draft.SegmentID, publication.ID, publication.Digest)
					request.Complete(found, err)
				case runs.ExecutionFactCommit:
					var err error
					if batch, ok := request.Fact().(runs.ToolResultsCommitted); ok {
						publications.Add(1)
						err = sqlite.RunInTx(ctx, db, func(ctx context.Context) error {
							for _, result := range batch.Results {
								if err := history.Write(ctx, draft.SessionID, chat.NewToolMessage(*result.ModelResult)); err != nil {
									return err
								}
							}
							return state.RecordResultPublication(ctx, draft.SessionID, draft.RunID, draft.SegmentID, batch.Publication.ID, batch.Publication.Digest)
						})
						if err == nil && replay == "lost acknowledgment" {
							err = errors.New("commit acknowledgment lost")
						}
					}
					request.Complete(err)
				}
			}
			committer := resultCommitterFunc(func(ctx context.Context, batch interaction.ResultBatch) (interaction.ResultReceipt, error) {
				first, err := session.CommitResults(ctx, batch)
				if replay == "lost acknowledgment" {
					if err == nil {
						return interaction.ResultReceipt{}, errors.New("lost acknowledgment was not reported")
					}
				} else if err != nil {
					return interaction.ResultReceipt{}, err
				}
				target := session
				if replay == "new projection host" {
					// Only the process identity survives; no Tool metadata or cached batch
					// is copied into the replacement product projection owner.
					target = &interactionSession{state: interactionState{process: session.state.processHandle()}, lifetime: newInteractionLifetime(ctx)}
					defer target.lifetime.beginRelease()
					done := make(chan struct{})
					go func() {
						defer close(done)
						for {
							select {
							case event := <-target.lifetime.events:
								handle(event)
							case <-target.lifetime.releasing:
								return
							}
						}
					}()
					defer func() { target.lifetime.beginRelease(); <-done }()
				}
				second, err := target.CommitResults(ctx, batch)
				if err != nil || second != batch.Receipt() || replay != "lost acknowledgment" && first != second {
					return interaction.ResultReceipt{}, fmt.Errorf("replayed receipt differs: first=%+v second=%+v: %w", first, second, err)
				}
				return second, nil
			})
			installTestResultCommitter(t, session, model, committer)
			sequence, err := executor.Observe(ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			finished := make(chan []runs.SegmentEnded, 1)
			go func() {
				var ends []runs.SegmentEnded
				for event := range sequence {
					handle(event)
					if end, ok := event.Payload.(runs.SegmentEnded); ok {
						ends = append(ends, end)
					}
				}
				finished <- ends
			}()
			if err := executor.BeginRoot(ctx, ref); err != nil {
				t.Fatal(err)
			}
			select {
			case ends := <-finished:
				if len(ends) != 1 || ends[0].Reason != run.OutcomeCompleted {
					t.Fatalf("replayed publication did not complete: %+v", ends)
				}
			case <-ctx.Done():
				t.Fatal("result publication did not settle")
			}
			if executions.Load() != 1 || modelCalls.Load() != 2 || publications.Load() != 1 {
				t.Fatalf("replay repeated work: tools=%d models=%d publications=%d", executions.Load(), modelCalls.Load(), publications.Load())
			}
			stored, err := history.Read(ctx, draft.SessionID)
			want := []chat.Message{chat.NewToolMessage(chat.ToolResult{ID: "store_once", Name: "store", Output: chat.NewTextToolOutput("stored once")})}
			if err != nil || !reflect.DeepEqual(stored, want) {
				t.Fatalf("replay changed history: %+v, %v", stored, err)
			}
		})
	}
}

type resultCommitterFunc func(context.Context, interaction.ResultBatch) (interaction.ResultReceipt, error)

func (f resultCommitterFunc) CommitResults(ctx context.Context, batch interaction.ResultBatch) (interaction.ResultReceipt, error) {
	return f(ctx, batch)
}

func installTestResultCommitter(t *testing.T, session *interactionSession, model chat.Model, committer interaction.ResultCommitter) {
	t.Helper()
	observed, err := newObservedInteractionModel(model, nil, session)
	if err != nil {
		t.Fatal(err)
	}
	definition := session.deployment.Definition().(*interaction.Definition)
	inner, err := interaction.NewDispatcher(definition, interaction.DispatcherConfig{
		Model: observed, ResultCommitter: committer,
		ModelContextReducer: newInteractionModelContextReducer(nil, nil, session, session.start, nil, nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition: definition, Dispatcher: &interactionDispatcher{inner: inner, session: session},
		ImplementationDigest: agent.ComputeDigest([]byte("result-replay-test")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("result-replay-test")),
	})
	if err != nil {
		t.Fatal(err)
	}
	session.deployment = deployment
	session.state.deployments.root = deployment
	session.state.deployments.byRef[deployment.DeploymentRef()] = deployment
}
