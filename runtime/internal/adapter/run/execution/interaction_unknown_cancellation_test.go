package execution

import (
	"context"
	"errors"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Keep one acknowledged unknown Effect, one pending storage acknowledgment,
// and one live sibling. Both stop requests must use Scope's ordering without
// waiting for unrelated work to become idle or discarding external evidence.
func TestUnknownEffectsRespectScopeCancellationOrderAndDrainSiblings(t *testing.T) {
	for _, unknownFirst := range []bool{false, true} {
		name := "operator first"
		if unknownFirst {
			name = "unknown first"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				settleStarted, workStarted := make(chan struct{}), make(chan struct{})
				settleRelease, workRelease := make(chan struct{}), make(chan struct{})
				workCanceled := make(chan struct{})
				releaseSettle := sync.OnceFunc(func() { close(settleRelease) })
				defer releaseSettle()
				releaseWork := sync.OnceFunc(func() { close(workRelease) })
				defer releaseWork()
				inner, err := toolcontract.NewFunc(toolcontract.FuncConfig{
					Name: "parallel", Description: "Run an external operation.",
				}, func(ctx context.Context, input struct {
					Operation string `json:"operation"`
				}) (string, error) {
					switch input.Operation {
					case "unknown":
						<-settleStarted
						<-workStarted
						return "", errors.New("connection closed after the external write")
					case "settle":
						close(settleStarted)
						<-settleRelease
						return "confirmed", nil
					default:
						close(workStarted)
						<-ctx.Done()
						close(workCanceled)
						<-workRelease
						return "", ctx.Err()
					}
				})
				if err != nil {
					t.Fatal(err)
				}
				model := &observationScriptModel{responses: []*chat.Response{
					interactionToolBatchResponse([]chat.ToolCall{
						{ID: "write", Name: "parallel", Arguments: `{"operation":"unknown"}`},
						{ID: "settle", Name: "parallel", Arguments: `{"operation":"settle"}`},
						{ID: "work", Name: "parallel", Arguments: `{"operation":"work"}`},
					}, 1, 1),
				}}
				store := &gatedExecutionTrees{
					ExecutionTreeStore: testTrees(t), entered: make(chan struct{}), release: make(chan struct{}),
				}
				releaseCommit := sync.OnceFunc(func() { close(store.release) })
				defer releaseCommit()
				executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
					ExecutionTrees: store,
					ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "parallel")}, manifest: toolset.Manifest{
						Visible: []toolcontract.Tool{concurrentInteractionTool{Tool: inner}},
					}},
					ToolInterpreter: immutableToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
					MaxConcurrentToolCalls: intPointer(3),
				})
				ref, err := executor.StageRoot(t.Context(), interactionTestStart())
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := executor.Release(context.Background(), ref); err != nil {
						t.Error(err)
					}
				})
				session, err := executor.session(ref)
				if err != nil {
					t.Fatal(err)
				}
				// Drive this existing reconciler explicitly after both requests can
				// be queued behind the same storage acknowledgment.
				session.lifetime.unknownWake = nil
				session.unknownPollInterval = time.Hour
				sequence, err := observeTestInteraction(t, executor, t.Context(), ref)
				if err != nil {
					t.Fatal(err)
				}
				eventsReady := collectInteractionEvents(sequence)
				if err := executor.BeginRoot(t.Context(), ref); err != nil {
					t.Fatal(err)
				}
				synctest.Wait()
				ids, err := session.unknownEffectIDs(t.Context())
				if err != nil || len(ids) != 1 {
					t.Fatalf("acknowledged unknown Effects = %v, error=%v", ids, err)
				}
				store.armed.Store(true)
				releaseSettle()
				<-store.entered
				cancel := func() {
					if err := executor.RequestRootCancellation(t.Context(), ref, "operator canceled"); err != nil {
						t.Fatal(err)
					}
				}
				if !unknownFirst {
					cancel()
				}
				reported := make(chan bool, 1)
				go func() { reported <- session.reportUnknownEffects() }()
				synctest.Wait()
				if unknownFirst {
					cancel()
				}
				releaseCommit()
				if !<-reported {
					t.Fatal("reconciler did not observe the acknowledged unknown Effect")
				}
				<-workCanceled
				synctest.Wait()
				select {
				case events := <-eventsReady:
					t.Fatalf("terminal abandoned an in-flight sibling: %+v", events)
				default:
				}
				releaseWork()
				events := <-eventsReady
				ends := payloadsOf[runs.SegmentEnded](events)
				wantOutcome, wantReason := run.OutcomeCanceled, "run cancellation: operator canceled"
				if unknownFirst {
					wantOutcome, wantReason = run.OutcomeLost, unresolvedEffectsStopReason
				}
				if len(ends) != 1 || ends[0].Reason != wantOutcome {
					t.Fatalf("terminal = %+v, want exactly one %s", ends, wantOutcome)
				}
				result, err := session.state.processHandle().Await(t.Context())
				if err != nil || result.Termination().Cause() != agent.TerminationCauseHostCancellation ||
					result.Termination().Reason() != wantReason {
					t.Fatalf("authoritative Scope termination = %+v, %v", result.Termination(), err)
				}
				found := false
				for _, effect := range ends[0].UnresolvedEffects() {
					if effect.EffectID() == ids[0].String() {
						found = true
						if effect.Cause() != agent.TerminationCauseParentCancellation.String() ||
							effect.Detail() != "connection closed after the external write" {
							t.Fatalf("unknown write evidence changed: %+v", effect)
						}
					}
				}
				if !found {
					t.Fatalf("terminal lost Effect %s: %+v", ids[0], ends[0].UnresolvedEffects())
				}
			})
		})
	}
}

func TestUserCancellationCannotImpersonateInternalStops(t *testing.T) {
	for _, test := range []struct {
		name   string
		reason string
	}{
		{name: "unknown stop text", reason: unresolvedEffectsStopReason},
		{name: "model stop text", reason: modelProcessStopReason},
		{name: "maximum unicode note", reason: strings.Repeat("🌋", runs.MaxCancellationReasonCharacters)},
	} {
		t.Run(test.name, func(t *testing.T) {
			started := make(chan struct{})
			model := chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			})
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{})
			events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), func() {
				<-started
				session := executor.sessions.snapshot()[0]
				if err := executor.RequestRootCancellation(t.Context(), session.ref, test.reason); err != nil {
					t.Fatal(err)
				}
			})
			ends := payloadsOf[runs.SegmentEnded](events)
			if len(ends) != 1 || ends[0].Reason != run.OutcomeCanceled || ends[0].Failure() != nil {
				t.Fatalf("user note changed cancellation semantics: %+v", ends)
			}
		})
	}
}

type gatedExecutionTrees struct {
	runs.ExecutionTreeStore
	armed   atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func (g *gatedExecutionTrees) SaveExecutionTree(ctx context.Context, update runs.ExecutionTreeUpdate) error {
	if g.armed.CompareAndSwap(true, false) {
		close(g.entered)
		select {
		case <-g.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return g.ExecutionTreeStore.SaveExecutionTree(ctx, update)
}
