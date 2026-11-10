package execution

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	"github.com/Tangerg/scope/core/chat"
	corecontent "github.com/Tangerg/scope/core/content"
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
				completed[0].ReportedUsage.InputTokens != 7 || completed[0].ReportedUsage.OutputTokens != 2 ||
				completed[0].FirstOutputLatencyMillis != nil {
				t.Fatalf("usage-only model completion = %+v", completed)
			}
			if len(payloadsOf[runs.ModelCallFailed](events)) != 0 || len(payloadsOf[runs.MessageDelta](events)) != 0 {
				t.Fatalf("valid response invented a failed invocation or message: %+v", events)
			}
			ends := payloadsOf[runs.SegmentEnded](events)
			if len(ends) != 1 || ends[0].Ending.Outcome() != run.OutcomeFailed || segmentFailure(ends[0]) == nil ||
				segmentFailure(ends[0]).Kind != run.FailureProviderRejected || len(ends[0].UnresolvedEffects()) != 0 {
				t.Fatalf("Scope rejection = %+v", ends)
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
	observed := toolcontract.Output{Content: []corecontent.Block{{Kind: corecontent.Text, Text: "observed before cancellation"}}, Details: []byte(`{"receipt":9007199254740993,"confirmed":false}`)}
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "fixture_block", Description: "Await an external operation."}, func(ctx context.Context, _ struct{}) (string, error) {
		close(entered)
		<-release
		failure, err := toolcontract.NewCallError(toolcontract.CallErrorConfig{Cause: errors.New("external result could not be confirmed"), Evidence: observed})
		if err != nil {
			return "", err
		}
		return "", failure
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{interactionToolResponse(chat.ToolCall{ID: "external", Name: "fixture_block", Arguments: `{}`}, 1, 1)}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "block")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}}, ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{}})
	events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), func() {
		<-entered
		session := executor.sessions.snapshot()[0]
		if err := executor.RequestRootCancellation(t.Context(), session.ref, "operator canceled"); err != nil {
			t.Error(err)
		}
		inspection, err := session.engine.Inspect(t.Context(), session.processRootID())
		if err != nil {
			t.Fatal(err)
		}
		root, _ := inspection.Process(session.processRootID())
		if root.Status != scoperun.StatusDraining || root.Termination.Valid() {
			t.Errorf("unconfirmed Tool cancellation became a terminal: %+v", root)
		}
		close(release)
	})
	ends := payloadsOf[runs.SegmentEnded](events)
	if len(ends) != 1 || ends[0].Ending.Outcome() != run.OutcomeLost {
		t.Fatalf("terminal = %+v", ends)
	}
	effects := ends[0].UnresolvedEffects()
	if len(effects) == 0 {
		t.Fatal("canceled external Tool lost its evidence")
	}
	found := false
	for _, effect := range effects {
		if strings.Contains(effect.Detail(), "external result") {
			var output toolcontract.Output
			if err := json.Unmarshal([]byte(effect.Output()), &output); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(output, observed) {
				t.Fatalf("Tool output evidence missing: got %+v, want %+v", effect.Output(), observed)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("Tool diagnostic missing: %+v", effects)
	}
	if results := payloadsOf[runs.ToolResultsCommitted](events); len(results) != 0 {
		t.Fatalf("unknown output was promoted to a model result: %+v", results)
	}
}

func TestInvalidToolCallErrorHasNoObservedOutput(t *testing.T) {
	for _, cause := range []error{&toolcontract.CallError{}, (*toolcontract.CallError)(nil)} {
		t.Run(fmt.Sprintf("%v", cause), func(t *testing.T) {
			executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "fixture_observe", Description: "Observe an external operation."}, func(context.Context, struct{}) (string, error) {
				return "", cause
			})
			if err != nil {
				t.Fatal(err)
			}
			model := &observationScriptModel{responses: []*chat.Response{interactionToolResponse(chat.ToolCall{ID: "external", Name: "fixture_observe", Arguments: `{}`}, 1, 1)}}
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "observe")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}}, ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{}})
			events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
			ends := payloadsOf[runs.SegmentEnded](events)
			if len(ends) != 1 || ends[0].Ending.Outcome() != run.OutcomeLost || len(ends[0].UnresolvedEffects()) == 0 {
				t.Fatalf("invalid error did not retain its unresolved outcome: %+v", ends)
			}
			for _, effect := range ends[0].UnresolvedEffects() {
				if effect.Output() != "" {
					t.Fatalf("invalid CallError invented an observed output: %s", effect.Output())
				}
			}
		})
	}
}

func TestModelResponseBudgetPreservesExternalBoundary(t *testing.T) {
	for _, test := range []struct {
		name              string
		limit             int
		invalidDefinition bool
		commitError       error
	}{
		{name: "stream exceeds budget", limit: 512},
		{name: "minimum response cannot fit", limit: 64, invalidDefinition: true},
		{name: "rejected stream observation cannot commit", limit: 512, commitError: errors.New("failed observation store unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := streamingObservationModel{chunks: test.limit * 2, streamed: make(chan struct{})}
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{StreamModelResponses: true})
			ref, err := executor.StageRoot(t.Context(), interactionTestStart())
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := executor.Release(context.Background(), ref); err != nil {
					t.Error(err)
				}
			}()
			session, err := executor.session(ref)
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := executor.resolveChat(t.Context(), session.start.ModelSelection)
			if err != nil {
				t.Fatal(err)
			}
			streamer, _ := resolved.Streamer()
			observed, err := newObservedInteractionModel(resolved.Model(), streamer, session)
			if err != nil {
				t.Fatal(err)
			}
			stream := interaction.StreamModelConfig{Streamer: observed}
			if test.commitError != nil {
				stream.MaxDeltaBytes = test.limit
			}
			replacement, err := interaction.Compile(session.engine.Compiler(), interaction.Config{
				Name: "stream_limit", Description: "Exercise model response capacity.", Revision: "stream-limit",
				Model: interaction.StreamModel(stream), MaxResponseBytes: test.limit,
				ContextProcessor: newInteractionModelContextReducer(nil, nil, session, session.start, nil, nil, nil),
			})
			if test.invalidDefinition {
				if !errors.Is(err, interaction.ErrInvalidDefinitionConfig) {
					t.Fatalf("unusable response budget admitted: %v", err)
				}
				select {
				case <-model.streamed:
					t.Fatal("provider was dispatched without a usable response budget")
				default:
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			session.deployment = replacement.Erase()
			session.state.deployments.root = session.deployment
			compiled, err := newCompiledInteraction(replacement)
			if err != nil {
				t.Fatal(err)
			}
			session.state.deployments.interactions = map[scoperun.ProgramRef]compiledInteraction{replacement.ProgramRef(): compiled}
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
			if len(ends) != 1 || ends[0].Ending.Outcome() != run.OutcomeLost {
				t.Fatalf("resource rejection = %+v", ends)
			}
			effects := ends[0].UnresolvedEffects()
			cause := interaction.ErrModelResponseTooLarge
			if test.commitError != nil {
				cause = test.commitError
			}
			if len(effects) != 1 || !strings.Contains(effects[0].Detail(), cause.Error()) {
				t.Fatalf("dispatch cause lost: %+v", effects)
			}
		})
	}
}

// TestCapacityStopAfterCompleteResponseRetainsKnownUsage guards R02: when the
// consumer stops reading on a chunk that already completed the physical stream,
// the single failure fact must carry the known response and its usage, not the
// unknown outcome the old inline settle recorded before the physical settled.
func TestCapacityStopAfterCompleteResponseRetainsKnownUsage(t *testing.T) {
	model := budgetStopCompleteModel{text: strings.Repeat("x", 2048), usage: &chat.Usage{InputTokens: 9, OutputTokens: 4}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{StreamModelResponses: true})
	ref, err := executor.StageRoot(t.Context(), interactionTestStart())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := executor.Release(context.Background(), ref); err != nil {
			t.Error(err)
		}
	}()
	session, err := executor.session(ref)
	if err != nil {
		t.Fatal(err)
	}
	// Build the budgeted Dispatcher over the session's journaled streamer so the
	// physical endpoint is recorded, then settled, before the capacity stop reads it.
	resolved, err := executor.resolveChat(t.Context(), session.start.ModelSelection)
	if err != nil {
		t.Fatal(err)
	}
	streamer, _ := resolved.Streamer()
	observed, err := newObservedInteractionModel(resolved.Model(), streamer, session)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := interaction.Compile(session.engine.Compiler(), interaction.Config{
		Name: "known_stop", Description: "Exercise known model response capacity.", Revision: "known-stop",
		Model:            interaction.StreamModel(interaction.StreamModelConfig{Streamer: observed, MaxDeltaBytes: 512}),
		ContextProcessor: newInteractionModelContextReducer(nil, nil, session, session.start, nil, nil, nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	session.deployment = replacement.Erase()
	session.state.deployments.root = session.deployment
	compiled, err := newCompiledInteraction(replacement)
	if err != nil {
		t.Fatal(err)
	}
	session.state.deployments.interactions = map[scoperun.ProgramRef]compiledInteraction{replacement.ProgramRef(): compiled}
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
				commit.Complete(nil)
			}
			events = append(events, event)
		}
		ready <- events
	}()
	if err := executor.BeginRoot(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	events := <-ready
	failed := payloadsOf[runs.ModelCallFailed](events)
	if len(failed) != 1 || failed[0].KnownResponse == nil || failed[0].Accounting == nil ||
		failed[0].Accounting.ReportedUsage == nil || failed[0].Accounting.ReportedUsage.InputTokens != 9 ||
		failed[0].Accounting.ReportedUsage.OutputTokens != 4 || len(payloadsOf[runs.ModelCallCompleted](events)) != 0 {
		t.Fatalf("capacity stop after a complete response dropped the known usage: %+v", failed)
	}
}

type budgetStopCompleteModel struct {
	text  string
	usage *chat.Usage
}

func (budgetStopCompleteModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return nil, errors.New("unexpected synchronous model call")
}

func (m budgetStopCompleteModel) Stream(context.Context, *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return func(yield func(*chat.ResponseDelta, error) bool) {
		yield(&chat.ResponseDelta{
			Parts:        []chat.PartDelta{chat.NewTextDelta(m.text)},
			Metadata:     &chat.ResponseMetadata{Model: "served-model", Usage: m.usage},
			FinishReason: chat.FinishReasonStop,
		}, nil)
	}
}

func scopeTestCoordinator(t *testing.T, executor *InteractionExecutor) testRuns {
	t.Helper()
	current := testsupport.MustRestoreSession(session.Snapshot{ID: "session_1", Title: "scope", Workspace: testsupport.MustWorkspace(t.TempDir())})
	projection := newDelegateProjection(t, current, executor)
	var id int
	coordinator := mustNewRunCoordinator(t, runTestDeps{
		RootStarts: executor, Observations: executor, Releases: executor, RootCancellation: executor,
		Conversation: projection.history, Session: projection.sessions,
		Projection: runTestProjection{Openings: projection, ChildStarts: projection, Events: projection, Barriers: projection, Checkpoints: projection, Workspace: projection, Finalizer: projection},
		Admissions: projection.admissions, Now: time.Now,
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
	executor := newProductInteractionExecutor(t, model, nil, InteractionExecutorConfig{})
	coordinator := scopeTestCoordinator(t, executor)
	selection, err := modelchoice.NewWithReasoningEffort("test-provider", "test-model", "high")
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
	executor := newProductInteractionExecutor(t, model, nil, InteractionExecutorConfig{})
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
		inspection, err := session.engine.Inspect(t.Context(), session.processRootID())
		if err != nil {
			t.Fatal(err)
		}
		root, _ := inspection.Process(session.processRootID())
		if root.Status == scoperun.StatusWaiting {
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
	executor := newProductInteractionExecutor(t, model, nil, InteractionExecutorConfig{})
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
		if outcome, _ := terminal.Outcome(); outcome != run.OutcomeLost {
			t.Fatalf("terminal %d: %+v", index, terminal)
		}
	}
	if terminals[2].Lineage().IsChild() || terminals[0].Lineage().ParentRunID() != terminals[1].ID() || terminals[1].Lineage().ParentRunID() != terminals[2].ID() {
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
			detach, attached := session.state.attachObserver(t.Context())
			if !attached {
				t.Fatal("observation was not admitted")
			}
			defer detach()
			defer session.lifetime.beginRelease()
			session.lifetime.stopExecution()
			finished := make(chan error, 1)
			go func() {
				finished <- session.commitFact(context.WithoutCancel(t.Context()), runs.ExecutorMember{MemberID: "root"}, runs.NewSegmentEnded(run.Canceled{}, 0))
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
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "fixture_ordered", Description: "Perform one ordered operation."}, func(ctx context.Context, input struct {
		Step int `json:"step"`
	}) (string, error) {
		started = append(started, input.Step)
		if input.Step == 2 {
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
		{ID: "first", Name: "fixture_ordered", Arguments: `{"step":1}`},
		{ID: "second", Name: "fixture_ordered", Arguments: `{"step":2}`},
		{ID: "third", Name: "fixture_ordered", Arguments: `{"step":3}`},
	}, 1, 1)}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{MaxConcurrentToolCalls: new(1), ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "ordered")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}}, ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{}})
	events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), func() {
		<-entered
		session := executor.sessions.snapshot()[0]
		inspection, err := session.engine.Inspect(t.Context(), session.processRootID())
		if err != nil {
			t.Fatal(err)
		}
		root, _ := inspection.Process(session.processRootID())
		for _, pending := range root.PendingEffects {
			intent, found, err := interaction.ToolCallFromEffect(pending.Request.Effect())
			if err != nil {
				t.Fatal(err)
			}
			if found && intent.Call.ID == "second" {
				unresolved = pending.Request.ID().String()
			}
		}
		if unresolved == "" {
			t.Fatal("second operation has no committed effect")
		}
		if err := executor.RequestRootCancellation(t.Context(), session.ref, "operator canceled"); err != nil {
			t.Error(err)
		}
		close(release)
		if _, err := session.state.process.Await(t.Context()); err == nil {
			t.Error("unresolved cancellation manufactured a terminal result")
		}
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
	if len(ends) != 1 || ends[0].Ending.Outcome() != run.OutcomeLost {
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
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "fixture_tick", Description: "Complete one operation."}, func(context.Context, struct {
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
			return interactionToolResponse(chat.ToolCall{ID: fmt.Sprintf("tick_%d", call), Name: "fixture_tick", Arguments: fmt.Sprintf(`{"index":%d}`, index)}, 1_000_000, 1_000), nil
		}
		if !child && call == 71 {
			return interactionToolResponse(chat.ToolCall{ID: "child", Name: "delegate_task", Arguments: `{"summary":"child","instructions":"long child"}`}, 1_000_000, 1_000), nil
		}
		if child {
			session := executor.sessions.snapshot()[0]
			history, err := session.engine.History(ctx, session.processRootID())
			if err != nil {
				return nil, err
			}
			for _, entry := range history.Entries {
				started, ok := entry.Record.(scoperun.Started)
				if !ok {
					continue
				}
				for _, quota := range []scoperun.Quota{started.Budget.Steps, started.Budget.Effects, started.Budget.Messages} {
					if _, limited := quota.Maximum(); limited {
						return nil, fmt.Errorf("process %s retains a cumulative quota", started.Process)
					}
				}
			}
			inspected = true
		}
		return interactionUsageTextResponse("finished", 1_000_000, 1_000), nil
	})
	executor = newProductInteractionExecutor(t, model, fixedInteractionPricing(100), InteractionExecutorConfig{ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "tick")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}}, ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{}})
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
		if !metered || usage.Total.InputTokens < 71_000_000 || usage.Total.Cost.OptionalUSD() == nil || *usage.Total.Cost.OptionalUSD() < 7100 {
			t.Fatalf("long execution lost metering: %+v", usage)
		}
		outcome, found := terminal.Run.Outcome()
		if !found || outcome != run.OutcomeCompleted {
			t.Fatalf("long execution did not complete: %+v", terminal)
		}
	}
}
