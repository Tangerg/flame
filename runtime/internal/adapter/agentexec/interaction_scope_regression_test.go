package agentexec

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestModelResponseWithoutMessageRetainsAccountingAndScopeFailure(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			executor := newObservedTestInteractionExecutor(t, usageOnlyModel{}, InteractionExecutorConfig{
				StreamModelResponses: stream,
			})
			events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
			completed := payloadsOf[runs.ModelCallCompleted](events)
			if len(completed) != 1 || completed[0].Message != nil || completed[0].ReportedUsage == nil ||
				completed[0].ReportedUsage.PromptTokens != 7 || completed[0].ReportedUsage.CompletionTokens != 2 ||
				completed[0].Steps != 1 || completed[0].FirstOutputLatencyMillis != nil {
				t.Fatalf("usage-only model completion = %+v", completed)
			}
			if len(payloadsOf[runs.ModelCallFailed](events)) != 0 || len(payloadsOf[runs.MessageDelta](events)) != 0 {
				t.Fatalf("valid response invented a failed invocation or message: %+v", events)
			}
			ends := payloadsOf[runs.SegmentEnded](events)
			if len(ends) != 1 || ends[0].Reason != run.OutcomeFailed || ends[0].Failure() == nil ||
				ends[0].Failure().Kind != run.FailureProviderRejected || len(ends[0].UnresolvedEffects()) != 0 {
				t.Fatalf("Scope rejection = %+v", ends)
			}
			if usage := ends[0].Usage(); usage == nil || usage.Steps != 1 || usage.Tokens.PromptTokens != 7 || usage.Tokens.CompletionTokens != 2 {
				t.Fatalf("terminal lost completed model usage: %+v", usage)
			}
		})
	}
}

type usageOnlyModel struct{}

func (usageOnlyModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return chat.NewResponse(&chat.Output{FinishReason: chat.FinishReasonStop}, &chat.ResponseMetadata{
		Model: "test-model", Usage: &chat.Usage{InputTokens: 7, OutputTokens: 2},
	})
}

func (u usageOnlyModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return testsupport.StreamResponse(u.Call(ctx, request))
}

func TestCanceledToolRetainsEvidenceAfterRootAwait(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "block", Description: "Await an external operation."}, func(ctx context.Context, _ struct{}) (string, error) {
		close(entered)
		<-release
		return "", errors.New("external result could not be confirmed")
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{interactionToolResponse(chat.ToolCall{ID: "external", Name: "block", Arguments: `{}`}, 1, 1)}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{ToolResolver: staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}}, ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{}})
	events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), func() {
		<-entered
		session := executor.sessions.snapshot()[0]
		if err := executor.RequestRootCancellation(t.Context(), session.ref, "operator canceled"); err != nil {
			t.Error(err)
		}
		result, err := session.state.process.Await(t.Context())
		if err != nil || !result.Status().Terminal() {
			t.Errorf("root Await: %v", err)
		}
		close(release)
	})
	ends := payloadsOf[runs.SegmentEnded](events)
	if len(ends) != 1 || ends[0].Reason != run.OutcomeCanceled {
		t.Fatalf("terminal = %+v", ends)
	}
	effects := ends[0].UnresolvedEffects()
	if len(effects) == 0 {
		t.Fatal("canceled external Tool lost its evidence")
	}
	found := false
	for _, effect := range effects {
		if strings.Contains(effect.Detail(), "external result") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Tool diagnostic missing: %+v", effects)
	}
}

func TestModelResponseBudgetPreservesExternalBoundary(t *testing.T) {
	for _, test := range []struct {
		name        string
		limit       int
		beforeCall  bool
		outcome     run.Outcome
		commitError error
	}{
		{name: "stream exceeds budget", limit: 512, outcome: run.OutcomeLost},
		{name: "minimum response cannot fit", limit: 64, beforeCall: true, outcome: run.OutcomeFailed},
		{name: "rejected stream observation cannot commit", limit: 512, outcome: run.OutcomeLost, commitError: errors.New("failed observation store unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := streamingObservationModel{chunks: 20, streamed: make(chan struct{})}
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{StreamModelResponses: true})
			ref, err := executor.StageRoot(t.Context(), interactionTestStart())
			if err != nil {
				t.Fatal(err)
			}
			session, err := executor.session(ref)
			if err != nil {
				t.Fatal(err)
			}
			// Replace only the public Dispatcher resource policy; retain Runtime's
			// model publication boundary.
			deployment := session.deployment
			definition := deployment.Definition().(*interaction.Definition)
			observed, err := newObservedInteractionModel(model, model, session)
			if err != nil {
				t.Fatal(err)
			}
			dispatcher, err := interaction.NewDispatcher(definition, interaction.DispatcherConfig{Streamer: observed, MaxResponseBytes: test.limit, ModelContextReducer: newInteractionModelContextReducer(nil, nil, session, session.start, nil, nil)})
			if err != nil {
				t.Fatal(err)
			}
			replacement, err := agent.NewDeployment(agent.DeploymentConfig{Definition: definition, Dispatcher: &interactionDispatcher{inner: dispatcher, session: session}, ImplementationDigest: agent.ComputeDigest([]byte("stream-limit")), ConfigurationDigest: agent.ComputeDigest([]byte("stream-limit"))})
			if err != nil {
				t.Fatal(err)
			}
			session.deployment = replacement
			session.state.deployments.root = replacement
			session.state.deployments.byRef[replacement.DeploymentRef()] = replacement
			sequence, err := observeTestInteraction(t, executor, t.Context(), ref)
			if err != nil {
				t.Fatal(err)
			}
			ready := make(chan []runs.ExecutorEvent, 1)
			go func() {
				var events []runs.ExecutorEvent
				for event := range sequence {
					if commit, ok := event.Payload.(runs.ExecutionFactCommit); ok {
						event.Payload = commit.Fact()
						if _, failed := event.Payload.(runs.ModelCallFailed); failed {
							commit.Complete(test.commitError)
						} else {
							commit.Complete(nil)
						}
					}
					events = append(events, event)
				}
				ready <- events
			}()
			if err := executor.BeginRoot(t.Context(), ref); err != nil {
				t.Fatal(err)
			}
			events := <-ready
			ends := payloadsOf[runs.SegmentEnded](events)
			if len(ends) != 1 || ends[0].Reason != test.outcome {
				t.Fatalf("resource rejection = %+v", ends)
			}
			effects := ends[0].UnresolvedEffects()
			if test.beforeCall {
				if len(effects) != 0 || len(payloadsOf[runs.ModelCallStarted](events)) != 0 {
					t.Fatalf("pre-call rejection recorded external work: effects=%+v events=%+v", effects, events)
				}
				if ends[0].Failure() == nil || !strings.Contains(ends[0].Failure().Detail, interaction.ErrModelResponseTooLarge.Error()) {
					t.Fatalf("pre-call rejection lost its cause: %+v", ends[0])
				}
				select {
				case <-model.streamed:
					t.Fatal("provider was dispatched without a usable response budget")
				default:
				}
			} else {
				cause := interaction.ErrModelResponseTooLarge
				if test.commitError != nil {
					cause = test.commitError
				}
				if len(effects) != 1 || !strings.Contains(effects[0].Detail(), cause.Error()) {
					t.Fatalf("dispatch cause lost: %+v", effects)
				}
			}
			if err := executor.Release(t.Context(), ref); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func scopeTestCoordinator(t *testing.T, executor *InteractionExecutor) *runs.Coordinator {
	t.Helper()
	sessions := &delegateSessionStore{value: testsupport.MustRestoreSession(session.Snapshot{ID: "session_1", Title: "scope", Workspace: testsupport.MustWorkspace(t.TempDir())})}
	projection := newDelegateProjection(t)
	var id int
	coordinator := mustNewRunCoordinator(t, runs.Dependencies{
		RootStarts: executor, Observations: executor, Releases: executor, RootCancellation: executor,
		Conversation: delegateConversation{}, Session: runs.SessionPorts{Reader: sessions, Creator: sessions, ActiveRuns: sessions},
		Projection: runs.ProjectionPorts{Openings: projection, ChildStarts: projection, Events: projection, Barriers: projection, Checkpoints: projection, Workspace: projection, Finalizer: projection},
		Admissions: testsupport.NewAdmissionGate(), Now: time.Now,
		NewRunID: func() string { id++; return fmt.Sprintf("run_%d", id) }, NewSegmentID: func() string { id++; return fmt.Sprintf("segment_%d", id) },
	})
	t.Cleanup(func() {
		coordinator.BeginShutdown()
		if err := coordinator.AwaitShutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return coordinator
}

func TestNestedDelegatesInheritGenerationPolicy(t *testing.T) {
	var requests []chat.Options
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		requests = append(requests, request.Options.Clone())
		if toolResult(request.Messages, "delegate_task") != "" || userMessagesContain(request.Messages, "second child") {
			return interactionUsageTextResponse("done", 2, 1), nil
		}
		instruction := "first child"
		if userMessagesContain(request.Messages, "first child") {
			instruction = "second child"
		}
		return interactionToolResponse(chat.ToolCall{ID: "delegate", Name: "delegate_task", Arguments: fmt.Sprintf(`{"summary":"child","instructions":%q}`, instruction)}, 2, 1), nil
	})
	executor := newTestInteractionExecutor(t, model)
	coordinator := scopeTestCoordinator(t, executor)
	selection, err := modelref.NewWithReasoningEffort("test-provider", "test-model", "high")
	if err != nil {
		t.Fatal(err)
	}
	started, err := coordinator.Start(t.Context(), runs.StartCommand{SessionID: "session_1", ModelSelection: selection, Capabilities: run.Capabilities{ChildRuns: true}, Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "root task"}}, Options: &chat.Options{Temperature: new(0.3), MaxOutputTokens: new(int64(1234)), Stop: []string{"ROOT_ONLY"}}})
	if err != nil {
		t.Fatal(err)
	}
	events := slices.Collect(started.Events)
	if len(requests) != 5 {
		t.Fatalf("nested requests=%d events=%+v", len(requests), events)
	}
	for index, options := range requests {
		if options.ReasoningEffort != chat.ReasoningEffort("high") || options.Temperature == nil || *options.Temperature != 0.3 || options.MaxOutputTokens == nil || *options.MaxOutputTokens != 1234 {
			t.Fatalf("request %d lost options: %+v", index, options)
		}
		if index > 0 && index < 4 && len(options.Stop) != 0 {
			t.Fatalf("child inherited root stop: %+v", options)
		}
	}
}

func TestFifthDelegateIsRejectedWhileFourRemainActive(t *testing.T) {
	var started atomic.Int32
	release := make(chan struct{})
	fourStarted := make(chan struct{})
	model := chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		if userMessagesContain(request.Messages, "blocked child") {
			n := started.Add(1)
			if n == 4 {
				close(fourStarted)
			}
			<-release
			return interactionUsageTextResponse("child done", 2, 1), nil
		}
		if toolResult(request.Messages, "delegate_task") != "" {
			return interactionUsageTextResponse("root done", 2, 1), nil
		}
		calls := make([]chat.ToolCall, 5)
		for index := range calls {
			calls[index] = chat.ToolCall{ID: fmt.Sprintf("delegate_%d", index), Name: "delegate_task", Arguments: `{"summary":"child","instructions":"blocked child"}`}
		}
		response := interactionToolResponse(calls[0], 2, 1)
		for _, call := range calls[1:] {
			response.Output.Message.Parts = append(response.Output.Message.Parts, chat.NewToolCallPart(call))
		}
		return response, nil
	})
	executor := newTestInteractionExecutor(t, model)
	coordinator := scopeTestCoordinator(t, executor)
	running, err := coordinator.Start(t.Context(), runs.StartCommand{SessionID: "session_1", Capabilities: run.Capabilities{ChildRuns: true}, Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "root task"}}})
	if err != nil {
		t.Fatal(err)
	}
	completed := make(chan []runs.Event, 1)
	go func() { completed <- slices.Collect(running.Events) }()
	<-fourStarted
	// The rejection is a model-visible Tool result. Keep all four accepted
	// children blocked until the fifth admission has been decided.
	session := executor.sessions.snapshot()[0]
	for {
		inspection, err := session.engine.InspectTree(t.Context(), session.processRootID())
		if err != nil {
			t.Fatal(err)
		}
		root, _ := inspection.Process(session.processRootID())
		if root.Snapshot.Status() == agent.StatusWaiting {
			break
		}
		runtime.Gosched()
	}
	close(release)
	events := <-completed
	if started.Load() != 4 {
		t.Fatalf("started %d delegates", started.Load())
	}
	children := 0
	for _, event := range events {
		if start, ok := event.Payload.(runs.SegmentStarted); ok && start.Run.Lineage().IsChild() {
			children++
		}
	}
	if children != 4 {
		t.Fatalf("admitted %d children", children)
	}
}

func TestJoinedDelegateTreePublishesEveryTerminalBeforeRoot(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	model := chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		if userMessagesContain(request.Messages, "grandchild") {
			close(entered)
			<-release
			return nil, ctx.Err()
		}
		instruction := "child"
		if userMessagesContain(request.Messages, "child") {
			instruction = "grandchild"
		}
		return interactionToolResponse(chat.ToolCall{ID: "delegate", Name: "delegate_task", Arguments: fmt.Sprintf(`{"summary":"delegate","instructions":%q}`, instruction)}, 2, 1), nil
	})
	executor := newTestInteractionExecutor(t, model)
	coordinator := scopeTestCoordinator(t, executor)
	started, err := coordinator.Start(t.Context(), runs.StartCommand{SessionID: "session_1", Capabilities: run.Capabilities{ChildRuns: true}, Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "root"}}})
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan []runs.Event, 1)
	go func() { ready <- slices.Collect(started.Events) }()
	<-entered
	session := executor.sessions.snapshot()[0]
	if err := executor.RequestRootCancellation(t.Context(), session.ref, "cancel tree"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.state.process.Await(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ready:
		t.Fatal("product tree ended before child drained")
	default:
	}
	close(release)
	var terminals []run.Run
	for _, event := range <-ready {
		if ended, ok := event.Payload.(runs.SegmentFinished); ok {
			terminals = append(terminals, ended.Run)
		}
	}
	if len(terminals) != 3 {
		t.Fatalf("terminal count=%d", len(terminals))
	}
	for index, terminal := range terminals {
		if outcome, _ := terminal.Outcome(); outcome != run.OutcomeCanceled {
			t.Fatalf("terminal %d: %+v", index, terminal)
		}
	}
	if terminals[2].Lineage().IsChild() || terminals[0].Lineage().ParentRunID != terminals[1].ID() || terminals[1].Lineage().ParentRunID != terminals[2].ID() {
		t.Fatalf("terminals are not in postorder: %+v", terminals)
	}
	if len(terminals[0].UnresolvedEffects()) != 1 {
		t.Fatalf("late child evidence missing: %+v", terminals[0].UnresolvedEffects())
	}
}

func TestTerminalReceiptFollowsPublicationLifetime(t *testing.T) {
	for _, release := range []bool{false, true} {
		t.Run(fmt.Sprintf("release=%t", release), func(t *testing.T) {
			session := &interactionSession{lifetime: newInteractionLifetime(t.Context())}
			defer session.lifetime.beginRelease()
			session.lifetime.stopExecution()
			finished := make(chan error, 1)
			go func() {
				finished <- session.commitFact(context.WithoutCancel(t.Context()), runs.ExecutorMember{MemberID: "root"}, runs.NewSegmentEnded(run.OutcomeCanceled, nil, nil, 0))
			}()
			event := <-session.lifetime.events
			commit := event.Payload.(runs.ExecutionFactCommit)
			select {
			case err := <-finished:
				t.Fatalf("execution cancellation abandoned terminal receipt: %v", err)
			default:
			}
			if release {
				session.lifetime.beginRelease()
				if err := <-finished; !errors.Is(err, context.Canceled) {
					t.Fatalf("release did not cancel receipt: %v", err)
				}
			} else {
				commit.Complete(nil)
				if err := <-finished; err != nil {
					t.Fatalf("durable receipt failed: %v", err)
				}
			}
		})
	}
}

func TestCancellationPreservesSettledPrefixAndDoesNotStartTail(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var started []int
	var unresolved string
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "ordered", Description: "Perform one ordered operation."}, func(ctx context.Context, input struct {
		Step int `json:"step"`
	}) (string, error) {
		started = append(started, input.Step)
		if input.Step == 2 {
			invocation, _ := interaction.ToolInvocationFromContext(ctx)
			unresolved = invocation.EffectID().String()
			close(entered)
			<-release
			return "", errors.New("second operation is unresolved")
		}
		return "confirmed", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{interactionToolBatchResponse([]chat.ToolCall{
		{ID: "first", Name: "ordered", Arguments: `{"step":1}`},
		{ID: "second", Name: "ordered", Arguments: `{"step":2}`},
		{ID: "third", Name: "ordered", Arguments: `{"step":3}`},
	}, 1, 1)}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{MaxConcurrentToolCalls: new(1), ToolResolver: staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}}, ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{}})
	events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), func() {
		<-entered
		session := executor.sessions.snapshot()[0]
		if err := executor.RequestRootCancellation(t.Context(), session.ref, "operator canceled"); err != nil {
			t.Error(err)
		}
		if _, err := session.state.process.Await(t.Context()); err != nil {
			t.Error(err)
		}
		close(release)
	})
	if !slices.Equal(started, []int{1, 2}) {
		t.Fatalf("unstarted tail executed: %v", started)
	}
	batches := payloadsOf[runs.ToolResultsCommitted](events)
	if len(batches) != 1 || len(batches[0].Results) != 1 {
		t.Fatalf("settled prefix was lost: %+v", batches)
	}
	known := batches[0].Results[0].ModelResult
	if known == nil || known.ID != "first" || known.IsError {
		t.Fatalf("settled prefix changed its result: %+v", known)
	}
	if output, ok := known.Output.Text(); !ok || output != "confirmed" {
		t.Fatalf("settled output = %q, %t", output, ok)
	}
	ends := payloadsOf[runs.SegmentEnded](events)
	if len(ends) != 1 || ends[0].Reason != run.OutcomeCanceled {
		t.Fatalf("terminal = %+v", ends)
	}
	effects := ends[0].UnresolvedEffects()
	if len(effects) != 1 || effects[0].EffectID() != unresolved {
		t.Fatalf("unresolved evidence includes a settled or unstarted operation: %+v", effects)
	}
}

func TestRootAndDelegateCompleteLongExecution(t *testing.T) {
	var rootCalls, childCalls, toolCalls atomic.Int32
	var executor *InteractionExecutor
	var inspected bool
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "tick", Description: "Complete one operation."}, func(context.Context, struct {
		Index int `json:"index"`
	}) (string, error) {
		toolCalls.Add(1)
		return "done", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		child := userMessagesContain(request.Messages, "long child")
		counter := &rootCalls
		if child {
			counter = &childCalls
		}
		call := counter.Add(1)
		if call <= 70 {
			index := call
			if child {
				index += 100
			}
			return interactionToolResponse(chat.ToolCall{ID: fmt.Sprintf("tick_%d", call), Name: "tick", Arguments: fmt.Sprintf(`{"index":%d}`, index)}, 1_000_000, 1_000), nil
		}
		if !child && call == 71 {
			return interactionToolResponse(chat.ToolCall{ID: "child", Name: "delegate_task", Arguments: `{"summary":"child","instructions":"long child"}`}, 1_000_000, 1_000), nil
		}
		if child {
			session := executor.sessions.snapshot()[0]
			tree, err := session.engine.InspectTree(ctx, session.processRootID())
			if err != nil {
				return nil, err
			}
			for _, member := range tree.Processes {
				budget := member.Snapshot.Budget()
				for _, quota := range []agent.Quota{budget.Steps, budget.Effects, budget.Signals} {
					if _, limited := quota.Maximum(); limited {
						return nil, fmt.Errorf("process %s retains a cumulative quota", member.Snapshot.ProcessID())
					}
				}
			}
			inspected = true
		}
		return interactionUsageTextResponse("finished", 1_000_000, 1_000), nil
	})
	executor = newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{Pricing: fixedInteractionPricing(100), ToolResolver: staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}}, ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{}})
	coordinator := scopeTestCoordinator(t, executor)
	started, err := coordinator.Start(t.Context(), runs.StartCommand{SessionID: "session_1", Capabilities: run.Capabilities{ChildRuns: true}, Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "long root"}}})
	if err != nil {
		t.Fatal(err)
	}
	var terminals []runs.SegmentFinished
	for event := range started.Events {
		if terminal, ok := event.Payload.(runs.SegmentFinished); ok {
			terminals = append(terminals, terminal)
		}
	}
	if !inspected || rootCalls.Load() != 72 || childCalls.Load() != 71 || toolCalls.Load() != 140 {
		t.Fatalf("execution stopped at an implicit cap: root=%d child=%d tools=%d terminals=%+v", rootCalls.Load(), childCalls.Load(), toolCalls.Load(), terminals)
	}
	if len(terminals) != 2 {
		t.Fatalf("terminals = %+v", terminals)
	}
	for _, terminal := range terminals {
		usage, metered := terminal.Run.Metrics().Usage()
		if !metered || usage.Total.InputTokens < 71_000_000 || usage.Total.CostUSD == nil || *usage.Total.CostUSD < 7100 {
			t.Fatalf("long execution lost metering: %+v", usage)
		}
		outcome, found := terminal.Run.Outcome()
		if !found || outcome != run.OutcomeCompleted {
			t.Fatalf("long execution did not complete: %+v", terminal)
		}
	}
}
