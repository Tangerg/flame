package execution

import (
	"context"
	"errors"
	"fmt"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"iter"
	"path/filepath"
	"testing"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient/safeguard"
)

type safeguardedTestModel struct {
	chat.Model
	chat.Streamer
}

func TestScopeDefiniteModelFailureSettlesWithoutHostCancellation(t *testing.T) {
	for _, scope := range []safeguard.Scope{safeguard.ScopeInput, safeguard.ScopeOutput} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/streaming=%t", scope, streaming), func(t *testing.T) {
				db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "evidence.db")))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Close() })
				journal := db.ModelInvocations
				var calls int
				physical := safeguardedTestModel{
					Model: chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
						calls++
						return interactionUsageTextResponse("blocked", 7, 3), nil
					}),
					Streamer: chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
						return func(yield func(*chat.ResponseDelta, error) bool) {
							calls++
							yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("blocked")}, FinishReason: chat.FinishReasonStop, Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: 7, OutputTokens: 3}}}, nil)
						}
					}),
				}
				recorded, err := modeladapter.NewObservedResolver(interactionChatResolver(physical, nil), fixtureEndpointJournal{ModelInvocationStore: journal, db: db}, nil)
				if err != nil {
					t.Fatal(err)
				}
				matcher, err := safeguard.NewSubstringMatcher([]string{"blocked"}, safeguard.SubstringConfig{})
				if err != nil {
					t.Fatal(err)
				}
				guard, err := safeguard.NewMiddleware(matcher, safeguard.MiddlewareConfig{Scope: scope})
				if err != nil {
					t.Fatal(err)
				}
				resolver := interactionChatResolverFunc(func(ctx context.Context, selection modelchoice.Selection) (modeladapter.ResolvedChat, error) {
					resolved, err := recorded.ResolveChat(ctx, selection)
					if err != nil {
						return modeladapter.ResolvedChat{}, err
					}
					stream, _ := resolved.Streamer()
					return modeladapter.NewResolvedChat(safeguardedTestModel{Model: guard.Call(resolved.Model()), Streamer: guard.Stream(stream)}, nil)
				})
				executor, err := newTestConfiguredInteractionExecutor(t, InteractionExecutorConfig{Lifetime: t.Context(), BuildID: interactionTestBuildID, ChatResolver: resolver, ModelEvidence: journal, StreamModelResponses: streaming})
				if err != nil {
					t.Fatal(err)
				}
				start := interactionTestStart()
				input := "clean"
				if scope == safeguard.ScopeInput {
					input = "blocked"
				}
				start.WorkingContext = []chat.Message{chat.NewUserMessage(chat.NewTextPart(input))}
				events := runInteractionHarness(t.Context(), t, executor, start, nil)
				ends := payloadsOf[runs.SegmentEnded](events)
				if len(ends) != 1 || ends[0].Reason != run.OutcomeFailed || len(ends[0].UnresolvedEffects()) != 0 {
					t.Fatalf("definite failure became unknown: %+v", ends)
				}
				failed := payloadsOf[runs.ModelCallFailed](events)
				if len(failed) != 1 || len(payloadsOf[runs.ModelCallCompleted](events)) != 0 {
					t.Fatalf("failed operation facts = %+v", failed)
				}
				if scope == safeguard.ScopeInput {
					if calls != 0 || failed[0].Accounting != nil {
						t.Fatalf("rejected input dispatched or counted a provider call: %d, %+v", calls, failed[0])
					}
				} else if calls != 1 || failed[0].Accounting == nil || failed[0].Accounting.ReportedUsage.InputTokens != 7 {
					t.Fatalf("rejected output lost original consumption: %d, %+v", calls, failed[0])
				}
				for _, session := range executor.sessions.snapshot() {
					result, err := session.state.process.Await(t.Context())
					if err != nil || result.Termination().Cause() != agent.TerminationCauseExternalFailure {
						t.Fatalf("Scope definite failure was replaced: %s, %v", result.Termination().Cause(), err)
					}
				}
			})
		}
	}
}

func TestScopeInvocationFailureDoesNotInheritNestedNonExecution(t *testing.T) {
	for _, kind := range []chat.CallFailureKind{chat.CallFailureFailed, chat.CallFailureUnknown} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/streaming=%t", kind, streaming), func(t *testing.T) {
				calls := 0
				failure := &chat.CallFailure{Kind: kind, Cause: &chat.CallFailure{
					Kind: chat.CallFailureNotExecuted, Cause: errors.New("later operation rejected"),
				}}
				model := safeguardedTestModel{
					Model: chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
						calls++
						return nil, failure
					}),
					Streamer: chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
						return func(yield func(*chat.ResponseDelta, error) bool) {
							calls++
							yield(nil, failure)
						}
					}),
				}
				executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{StreamModelResponses: streaming})
				events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
				failed := payloadsOf[runs.ModelCallFailed](events)
				if calls != 1 || len(failed) != 1 || failed[0].Accounting == nil || failed[0].Accounting.ReportedUsage != nil {
					t.Fatalf("nested non-execution replaced dispatched evidence: calls=%d, failures=%+v", calls, failed)
				}
				ends := payloadsOf[runs.SegmentEnded](events)
				if len(ends) != 1 || ends[0].Reason != run.OutcomeFailed {
					t.Fatalf("invocation failure became success: %+v", ends)
				}
				if unresolved := len(ends[0].UnresolvedEffects()); (unresolved > 0) != (kind == chat.CallFailureUnknown) {
					t.Fatalf("nested evidence changed invocation %s: unresolved=%d", kind, unresolved)
				}
			})
		}
	}
}
