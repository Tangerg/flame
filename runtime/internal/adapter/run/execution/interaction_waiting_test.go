package execution

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/testsupport"

	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/builtin"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	infraexec "github.com/Tangerg/flame/runtime/internal/infra/process/exec"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestInteractionExecutorRestoresWaitingTreeAndDeliversSemanticAnswer(t *testing.T) {
	workspace := t.TempDir()
	var (
		toolCalls int
		requests  [][]chat.Message
		mu        sync.Mutex
	)
	question, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "ask", Description: "Ask for one value before completing.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		resolution, err := runinput.Require(ctx, "question.ask", runs.Interrupt{
			Kind: interrupt.Question,
			Question: &runs.QuestionPrompt{
				ToolName: "ask", Arguments: `{}`,
				Fields: []runs.QuestionFieldSpec{{Prompt: "Which value?"}},
			},
		})
		if err != nil {
			return "", err
		}
		toolCalls++
		return resolution.Answers[0][0], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "ask_call", Name: "ask", Arguments: `{}`}, 2, 1),
		interactionUsageTextResponse("completed", 3, 1),
	}}
	recordingModel := chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		if request.Options.Temperature == nil || *request.Options.Temperature != 0.27 || request.Options.MaxOutputTokens == nil || *request.Options.MaxOutputTokens != 321 {
			return nil, errors.New("restoration lost generation options")
		}
		mu.Lock()
		requests = append(requests, cloneChatMessages(request.Messages))
		mu.Unlock()
		return model.Call(ctx, request)
	})
	compactor := &calibrationCaptureCompactor{estimatedTokens: 100}
	executor := newObservedTestInteractionExecutor(t, recordingModel, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "ask")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{question}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
		ModelContextCompactor: compactor, ModelContextState: emptyInteractionModelContextState{},
	})
	start := interactionTestStart()
	start.Options = &chat.Options{Temperature: new(0.27), MaxOutputTokens: new(int64(321))}
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Question}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	first, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error {
		return executor.BeginRoot(t.Context(), ref)
	})
	if len(payloadsOf[runs.ToolCallStarted](first)) != 1 || toolCalls != 0 {
		t.Fatalf("before answer starts=%d Tool calls=%d", len(payloadsOf[runs.ToolCallStarted](first)), toolCalls)
	}
	if releaseErr := executor.Release(t.Context(), ref); releaseErr != nil {
		t.Fatal(releaseErr)
	}

	continuation := rootInteractionWaitingContinuation(
		barrier.Checkpoint(),
		workspace,
		ref.ExecutorID,
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}},
	)
	resumption, err := executor.CanResumeWaitingExecution(t.Context(), continuation)
	resumable := resumption.Resumable()
	if err != nil || !resumable {
		t.Fatalf("CanResumeWaitingExecution with generation options = %t, %v, want true", resumable, err)
	}

	restoredWaiting, err := executor.RestoreWaitingExecution(t.Context(), continuation)
	if err != nil {
		t.Fatalf("RestoreWaitingExecution: %v", err)
	}
	if restoredWaiting != ref {
		t.Fatalf("restored waiting ref = %+v, want %+v", restoredWaiting, ref)
	}
	if _, restoreWaitingExecutionErr := executor.RestoreWaitingExecution(t.Context(), continuation); !errors.Is(restoreWaitingExecutionErr, runs.ErrExecutionClaimed) {
		t.Fatalf("second RestoreWaitingExecution error = %v, want ErrExecutionClaimed", restoreWaitingExecutionErr)
	}
	if releaseErr := executor.Release(t.Context(), restoredWaiting); releaseErr != nil {
		t.Fatal(releaseErr)
	}
	restored, err := executor.StageContinuation(t.Context(), continuation)
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := observeTestInteraction(t, executor, context.Background(), restored)
	if err != nil {
		t.Fatal(err)
	}
	resumedEvents := collectInteractionEvents(sequence)
	binding := barrier.Interruptions()[0]
	answers := []runs.InterruptAnswer{{
		InterruptItemID: "item_question", MemberID: binding.MemberID,
		RequestID:  binding.RequestID,
		Resolution: interrupt.Resolution{Answers: [][]string{{"chosen"}}},
	}}
	committedInput := &runs.CommittedUserInput{
		ItemID: "item_followup",
		Content: []transcript.ContentBlock{{
			Kind: transcript.TextContent, Text: "also include edge cases",
		}},
	}
	if err := executor.BeginContinuation(t.Context(), restored, answers, committedInput); err != nil {
		t.Fatal(err)
	}
	events := <-resumedEvents
	if toolCalls != 1 {
		var failure run.Failure
		if terminal := payloadsOf[runs.SegmentEnded](events); len(terminal) == 1 && terminal[0].Failure() != nil {
			failure = *terminal[0].Failure()
		}
		t.Fatalf("Tool calls after answer = %d, want 1; failure=%+v events=%#v", toolCalls, failure, events)
	}
	completed := payloadsOf[runs.ModelCallCompleted](events)
	if len(completed) != 1 {
		t.Fatalf("completion = %#v", completed)
	}
	message := completed[0].Message
	if message.Text() != "completed" {
		t.Fatalf("completion = %#v", completed)
	}
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted || ended[0].Usage() == nil || ended[0].Usage().Steps != 2 {
		t.Fatalf("terminal = %#v", ended)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 || requests[0][0].Text() != "current question" || requests[1][0].Text() != "current question" {
		t.Fatalf("restored model context = %#v", requests)
	}
	if got := requests[1][len(requests[1])-1].Text(); got != "also include edge cases" {
		t.Fatalf("continuation follow-up = %q, want committed input", got)
	}
	steers := payloadsOf[runs.SteerMessagesApplied](events)
	if len(steers) != 1 || len(steers[0].Messages) != 1 ||
		steers[0].Messages[0].ItemID != committedInput.ItemID ||
		len(steers[0].Messages[0].Content) != 1 || steers[0].Messages[0].Content[0].Text != "also include edge cases" {
		t.Fatalf("committed continuation input projection = %#v", steers)
	}
	if !reflect.DeepEqual(compactor.adjustments, []int{0, -98}) {
		t.Fatalf("restored model context calibration = %v, want [0 -98]", compactor.adjustments)
	}
	if err := executor.Release(t.Context(), restored); err != nil {
		t.Fatal(err)
	}
}

func TestInteractionExecutorRestoresRuntimeAskUserTool(t *testing.T) {
	workspace := t.TempDir()
	ask, err := builtin.NewAskUser(runinput.Require)
	if err != nil {
		t.Fatal(err)
	}
	arguments := `{"questions":[{"question":"Which value?"}]}`
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{
			ID: "ask_user_call", Name: string(domaintool.AskUser), Arguments: arguments,
		}, 1, 1),
		interactionUsageTextResponse("continued after the answer", 1, 1),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.BuiltInTool(t, "ask_user")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{ask}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Question}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	beforeAnswer, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error {
		return executor.BeginRoot(t.Context(), ref)
	})
	starts := payloadsOf[runs.ToolCallStarted](beforeAnswer)
	interruptions := barrier.Interruptions()
	if len(interruptions) != 1 || interruptions[0].Interrupt.Question == nil ||
		interruptions[0].Interrupt.Question.ToolName != string(domaintool.AskUser) ||
		len(starts) != 1 {
		t.Fatalf("ask_user interruption = %#v", interruptions)
	}
	if releaseErr := executor.Release(t.Context(), ref); releaseErr != nil {
		t.Fatal(releaseErr)
	}
	if _, stageContinuationErr := executor.StageContinuation(t.Context(), rootInteractionWaitingContinuation(
		barrier.Checkpoint(),
		workspace,
		ref.ExecutorID,
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}},
	)); stageContinuationErr != nil {
		t.Fatal(stageContinuationErr)
	}
	sequence, err := observeTestInteraction(t, executor, context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	events := collectInteractionEvents(sequence)
	binding := barrier.Interruptions()[0]
	if err := executor.BeginContinuation(t.Context(), ref, []runs.InterruptAnswer{{
		InterruptItemID: "item_ask_user", MemberID: binding.MemberID,
		RequestID:  binding.RequestID,
		Resolution: interrupt.Resolution{Answers: [][]string{{"chosen"}}},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	observed := <-events
	resumed := payloadsOf[runs.ToolCallStarted](observed)
	finished := payloadsOf[runs.ToolCallFinished](observed)
	ends := payloadsOf[runs.SegmentEnded](observed)
	if len(resumed) != 1 || resumed[0].CallID != starts[0].CallID ||
		len(finished) != 1 || finished[0].CallID != starts[0].CallID ||
		len(payloadsOf[runs.ModelCallCompleted](observed)) != 1 ||
		len(ends) != 1 || ends[0].Reason != run.OutcomeCompleted {
		t.Fatalf("restored ask_user lifecycle = %#v", observed)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}

func TestInteractionExecutorRestoresInteractiveApprovalWithoutRepeatingPolicyOrHook(t *testing.T) {
	workspace := t.TempDir()
	var toolCalls int
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "mutate", Description: "Perform an approved mutation.",
	}, func(context.Context, struct{}) (string, error) {
		toolCalls++
		return "mutated", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "mutate_call", Name: "mutate", Arguments: `{}`}, 1, 1),
		interactionUsageTextResponse("approved", 1, 1),
	}}
	hooks := &recordingInteractionHooks{}
	approvals := &promptingInteractionAuthorizer{}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "mutate")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: approvals, ToolHooks: hooks,
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Approval}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	first, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error {
		return executor.BeginRoot(t.Context(), ref)
	})
	if len(payloadsOf[runs.ToolCallStarted](first)) != 0 || toolCalls != 0 || hooks.before != 1 || approvals.planned != 1 {
		t.Fatalf("before approval starts=%d calls=%d hooks=%d plans=%d", len(payloadsOf[runs.ToolCallStarted](first)), toolCalls, hooks.before, approvals.planned)
	}
	if releaseErr := executor.Release(t.Context(), ref); releaseErr != nil {
		t.Fatal(releaseErr)
	}
	continuation := rootInteractionWaitingContinuation(
		barrier.Checkpoint(),
		workspace,
		ref.ExecutorID,
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval}},
	)

	if _, stageContinuationErr := executor.StageContinuation(t.Context(), continuation); stageContinuationErr != nil {
		t.Fatal(stageContinuationErr)
	}
	sequence, err := observeTestInteraction(t, executor, context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	events := collectInteractionEvents(sequence)
	binding := barrier.Interruptions()[0]
	if err := executor.BeginContinuation(t.Context(), ref, []runs.InterruptAnswer{{
		InterruptItemID: "item_approval", MemberID: binding.MemberID,
		RequestID:  binding.RequestID,
		Resolution: interrupt.Resolution{Approved: true},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	observed := <-events
	if toolCalls != 1 || hooks.before != 1 || hooks.after != 1 || approvals.planned != 1 || approvals.resolved != 1 {
		t.Fatalf("after approval calls=%d before=%d after=%d plans=%d resolves=%d events=%#v", toolCalls, hooks.before, hooks.after, approvals.planned, approvals.resolved, observed)
	}
	if len(payloadsOf[runs.ToolCallStarted](observed)) != 1 || len(payloadsOf[runs.ToolCallFinished](observed)) != 1 {
		t.Fatalf("approved Tool lifecycle = %#v", observed)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}

func TestInteractionExecutorCancellationStopsApprovedInflightTool(t *testing.T) {
	workspace := t.TempDir()
	toolStarted := make(chan struct{})
	toolReturned := make(chan struct{})
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "mutate", Description: "Perform an approved mutation until canceled.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		close(toolStarted)
		<-ctx.Done()
		close(toolReturned)
		return "", ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "mutate_call", Name: "mutate", Arguments: `{}`}, 1, 1),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "mutate")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: &promptingInteractionAuthorizer{},
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Approval}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	_, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error {
		return executor.BeginRoot(t.Context(), ref)
	})
	continuation := rootInteractionWaitingContinuation(
		barrier.Checkpoint(),
		workspace,
		ref.ExecutorID,
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval}},
	)
	if _, stageContinuationErr := executor.StageContinuation(t.Context(), continuation); stageContinuationErr != nil {
		t.Fatal(stageContinuationErr)
	}
	sequence, err := observeTestInteraction(t, executor, context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	events := collectInteractionEvents(sequence)
	binding := barrier.Interruptions()[0]
	if err := executor.BeginContinuation(t.Context(), ref, []runs.InterruptAnswer{{
		InterruptItemID: "item_approval", MemberID: binding.MemberID,
		RequestID: binding.RequestID, Resolution: interrupt.Resolution{Approved: true},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-toolStarted:
	case <-time.After(waitBudget(time.Second)):
		t.Fatal("approved Tool did not start")
	}
	if err := executor.RequestRootCancellation(t.Context(), ref, "operator canceled"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-toolReturned:
	case <-time.After(waitBudget(time.Second)):
		t.Fatal("approved Tool context was not canceled")
	}
	observed := <-events
	ended := payloadsOf[runs.SegmentEnded](observed)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCanceled {
		t.Fatalf("segment end = %#v, want canceled", ended)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}

func TestInteractionExecutorCancellationStopsApprovedForegroundShell(t *testing.T) {
	workspace := t.TempDir()
	shells := infraexec.NewShells(nil, false)
	t.Cleanup(func() {
		if err := shells.KillAll(); err != nil {
			t.Errorf("KillAll: %v", err)
		}
	})
	shellTools, err := builtin.BuildShell(shells)
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{
			ID: "shell_call", Name: string(domaintool.Shell),
			Arguments: `{"command":"sleep 30","description":"Wait until canceled"}`,
		}, 1, 1),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.BuiltInTool(t, "shell"), testsupport.BuiltInTool(t, "read_shell_output"), testsupport.BuiltInTool(t, "stop_shell")}, manifest: toolset.Manifest{Visible: shellTools}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: &promptingInteractionAuthorizer{},
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Approval}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	_, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error {
		return executor.BeginRoot(t.Context(), ref)
	})
	continuation := rootInteractionWaitingContinuation(
		barrier.Checkpoint(),
		workspace,
		ref.ExecutorID,
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval}},
	)
	if _, stageContinuationErr := executor.StageContinuation(t.Context(), continuation); stageContinuationErr != nil {
		t.Fatal(stageContinuationErr)
	}
	sequence, err := observeTestInteraction(t, executor, context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	events := collectInteractionEvents(sequence)
	binding := barrier.Interruptions()[0]
	if err := executor.BeginContinuation(t.Context(), ref, []runs.InterruptAnswer{{
		InterruptItemID: "item_approval", MemberID: binding.MemberID,
		RequestID: binding.RequestID, Resolution: interrupt.Resolution{Approved: true},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for len(shells.RetainedForSession(start.SessionID)) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if running := shells.RetainedForSession(start.SessionID); len(running) != 1 {
		t.Fatalf("running shells = %#v, want one approved foreground command", running)
	}
	if err := executor.RequestRootCancellation(t.Context(), ref, "operator canceled"); err != nil {
		t.Fatal(err)
	}
	observed := <-events
	ended := payloadsOf[runs.SegmentEnded](observed)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCanceled {
		t.Fatalf("segment end = %#v, want canceled", ended)
	}
	if running := shells.RetainedForSession(start.SessionID); len(running) != 0 {
		t.Fatalf("approved foreground shell survived cancellation: %#v", running)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}

func TestInteractionExecutorPreservesDeferredAdvertisementAcrossWaitingRestore(t *testing.T) {
	workspace := t.TempDir()
	hidden, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "hidden_lookup", Description: "Read a deferred value.",
	}, func(context.Context, struct{}) (string, error) { return "hidden", nil })
	if err != nil {
		t.Fatal(err)
	}
	identifiedHidden, err := toolset.WithIdentity(hidden, testsupport.A2ATool(t, "hidden_lookup"), testsupport.ToolFingerprint(testsupport.A2ATool(t, "hidden_lookup")))
	if err != nil {
		t.Fatal(err)
	}
	search, err := toolset.NewDiscovery([]toolcontract.Tool{identifiedHidden})
	if err != nil {
		t.Fatal(err)
	}
	question, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "ask", Description: "Ask before continuing.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		resolution, requireErr := runinput.Require(ctx, "question.after-search", runs.Interrupt{
			Kind: interrupt.Question,
			Question: &runs.QuestionPrompt{
				ToolName: "ask", Arguments: `{}`,
				Fields: []runs.QuestionFieldSpec{{Prompt: "Continue?"}},
			},
		})
		if requireErr != nil {
			return "", requireErr
		}
		return resolution.Answers[0][0], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &advertisementWaitingModel{}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.BuiltInTool(t, "search_tools"), testsupport.A2ATool(t, "ask")}, manifest: toolset.Manifest{
			Visible: []toolcontract.Tool{search, question}, Deferred: []toolcontract.Tool{identifiedHidden},
		}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Question}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	_, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error {
		return executor.BeginRoot(t.Context(), ref)
	})
	if releaseErr := executor.Release(t.Context(), ref); releaseErr != nil {
		t.Fatal(releaseErr)
	}
	if _, stageContinuationErr := executor.StageContinuation(t.Context(), rootInteractionWaitingContinuation(
		barrier.Checkpoint(),
		workspace,
		ref.ExecutorID,
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}},
	)); stageContinuationErr != nil {
		t.Fatal(stageContinuationErr)
	}
	sequence, err := observeTestInteraction(t, executor, context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	events := collectInteractionEvents(sequence)
	binding := barrier.Interruptions()[0]
	if err := executor.BeginContinuation(t.Context(), ref, []runs.InterruptAnswer{{
		InterruptItemID: "item_question", MemberID: binding.MemberID,
		RequestID:  binding.RequestID,
		Resolution: interrupt.Resolution{Answers: [][]string{{"yes"}}},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	observed := <-events
	ended := payloadsOf[runs.SegmentEnded](observed)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted {
		t.Fatalf("terminal = %#v", ended)
	}
	if !model.restoredManifestIncludedHidden {
		t.Fatal("restored model request omitted the previously advertised Tool")
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}

type advertisementWaitingModel struct {
	mu                             sync.Mutex
	calls                          int
	restoredManifestIncludedHidden bool
}

func (a *advertisementWaitingModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	switch a.calls {
	case 1:
		return interactionToolResponse(chat.ToolCall{
			ID: "discover", Name: "search_tools", Arguments: `{"query":"select:hidden_lookup"}`,
		}, 1, 1), nil
	case 2:
		return interactionToolResponse(chat.ToolCall{ID: "ask", Name: "ask", Arguments: `{}`}, 1, 1), nil
	case 3:
		for _, definition := range request.Tools {
			if definition.Name == "hidden_lookup" {
				a.restoredManifestIncludedHidden = true
			}
		}
		if !a.restoredManifestIncludedHidden {
			return nil, errors.New("deferred advertisement was lost across restore")
		}
		return interactionUsageTextResponse("done", 1, 1), nil
	default:
		return nil, errors.New("unexpected model call")
	}
}

type promptingInteractionAuthorizer struct {
	planned  int
	resolved int
}

// rootRunMetricsForCheckpoint stands in for the parked root Run, which owns
// the member's usage. Only the per-model call counts must agree with the
// checkpoint; the token values are the Run's own.
func rootRunMetricsForCheckpoint(checkpoint runs.ExecutorCheckpoint) run.Metrics {
	state, err := decodeExecutorCheckpoint(checkpoint)
	if err != nil {
		panic(err)
	}
	calls := state.callsByProcess[state.tree.RootID()]
	if len(calls) == 0 {
		return run.Metrics{}
	}
	usage := &accounting.Usage{ByModel: make(map[string]accounting.Totals, len(calls))}
	steps := 0
	for model, count := range calls {
		usage.ByModel[model] = accounting.Totals{}
		steps += count
	}
	metrics, err := run.NewMetrics(usage, steps, 0)
	if err != nil {
		panic(err)
	}
	return metrics
}

func rootInteractionWaitingContinuation(
	checkpoint runs.ExecutorCheckpoint,
	workspace string,
	executorID string,
	capabilities run.Capabilities,
) runs.WaitingContinuation {
	const rootRunID = "run_root"

	metrics := rootRunMetricsForCheckpoint(checkpoint)
	return runs.WaitingContinuation{
		SessionID: checkpoint.SessionID, ExecutorID: executorID, RootRunID: rootRunID,
		Members: []runs.WaitingMember{{
			RunID: rootRunID, MemberID: checkpoint.RootMemberID,
			ModelSelection: interactionTestStart().ModelSelection, Metrics: metrics,
		}},
		Checkpoint: checkpoint, Capabilities: capabilities, Workspace: workspace,
	}
}

func (p *promptingInteractionAuthorizer) AuthorizeTool(
	_ context.Context,
	request ToolAuthorizationRequest,
) (ToolAuthorizationDecision, error) {
	p.planned++
	prompt := runs.ApprovalPrompt{
		CallID: request.CallID, Arguments: request.Arguments.Canonical(),
		SafetyClass: request.SafetyClass, Risk: domaintool.RiskHigh,
		Reason: "This Tool changes external state.", Rememberable: true, Tool: request.Tool, SourceFingerprint: request.SourceFingerprint,
	}
	return AskToolApproval(prompt)
}

func (p *promptingInteractionAuthorizer) ResolveToolApproval(
	_ context.Context,
	request ToolAuthorizationRequest,
	_ runs.ApprovalPrompt,
	resolution interrupt.Resolution,
) (ToolAuthorizationDecision, error) {
	p.resolved++
	if !resolution.Approved {
		return DenyTool("denied by user"), nil
	}
	if resolution.Arguments == "" {
		return AllowTool(), nil
	}
	arguments, err := domaintool.ParseArguments(resolution.Arguments)
	if err != nil {
		return AllowTool(), err
	}
	if request.Tool.ModelName() == "" {
		return AllowTool(), errors.New("missing Tool identity")
	}
	return AllowToolWithArguments(arguments), nil
}

func TestInteractionExecutorRejectsInvalidWaitingRecoveryFacts(t *testing.T) {
	workspace := t.TempDir()
	checkpoint := captureInteractionQuestionCheckpoint(t, workspace)
	for _, test := range []struct {
		name   string
		mutate func(*runs.WaitingContinuation)
	}{
		{name: "corrupt payload", mutate: func(continuation *runs.WaitingContinuation) {
			continuation.Checkpoint.Payload = []byte(`{"tree":null}`)
		}},
		{name: "wrong build", mutate: func(continuation *runs.WaitingContinuation) {
			continuation.Checkpoint.BuildID = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		}},
		{name: "missing workspace", mutate: func(continuation *runs.WaitingContinuation) {
			continuation.Workspace = workspace + "/gone"
		}},
		{name: "isolated workspace", mutate: func(continuation *runs.WaitingContinuation) {
			continuation.Isolated = true
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				return nil, errors.New("model must not be called while restoring")
			}), InteractionExecutorConfig{})
			continuation := rootInteractionWaitingContinuation(
				checkpoint.Clone(),
				workspace,
				"exec_restore",
				run.Capabilities{},
			)
			test.mutate(&continuation)

			_, err := executor.StageContinuation(t.Context(), continuation)
			if !errors.Is(err, runs.ErrExecutorStateLost) {
				t.Fatalf("StageContinuation error = %v, want ErrExecutorStateLost", err)
			}
		})
	}
	t.Run("wrong deployment", func(t *testing.T) {
		model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
			return nil, errors.New("model must not be called while restoring")
		})
		executor, err := newTestConfiguredInteractionExecutor(t, InteractionExecutorConfig{
			Lifetime:     t.Context(),
			ChatResolver: staticInteractionChatResolver(model), BuildID: interactionTestBuildID,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = executor.StageContinuation(t.Context(), rootInteractionWaitingContinuation(
			checkpoint,
			workspace,
			"exec_restore",
			run.Capabilities{},
		))
		if !errors.Is(err, runs.ErrExecutorStateLost) {
			t.Fatalf("StageContinuation error = %v, want ErrExecutorStateLost", err)
		}
	})
}

func TestInteractionExecutorProbesWaitingCheckpointThroughExactRestorePath(t *testing.T) {
	workspace := t.TempDir()
	checkpoint := captureInteractionQuestionCheckpoint(t, workspace)
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("model must not be called while probing a waiting checkpoint")
	}), InteractionExecutorConfig{
		ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "ask")}, manifest: toolset.Manifest{
			Visible: []toolcontract.Tool{newQuestionCheckpointTool(t)},
		}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	continuation := rootInteractionWaitingContinuation(
		checkpoint,
		workspace,
		"exec_probe",
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}},
	)
	head, found, err := executor.config.ExecutionTrees.LoadExecutionTree(t.Context(), continuation.SessionID, checkpoint.RootMemberID)
	if err != nil || !found {
		t.Fatalf("LoadExecutionTree before probe = %t, %v", found, err)
	}
	resumption, err := executor.CanResumeWaitingExecution(t.Context(), continuation)
	resumable := resumption.Resumable()
	if err != nil || !resumable {
		t.Fatalf("CanResumeWaitingExecution = %t, %v, want true", resumable, err)
	}

	foreign := continuation
	foreign.Members = append([]runs.WaitingMember(nil), continuation.Members...)
	foreign.Members[0].MemberID = "interaction:foreign-member"
	resumption, err = executor.CanResumeWaitingExecution(t.Context(), foreign)
	resumable = resumption.Resumable()
	if err != nil || resumable || resumption.Loss() != runs.LossWaitingStateUnavailable {
		t.Fatalf("foreign CanResumeWaitingExecution = %+v, %v, want lost waiting state", resumption, err)
	}
	after, found, err := executor.config.ExecutionTrees.LoadExecutionTree(t.Context(), continuation.SessionID, checkpoint.RootMemberID)
	if err != nil || !found || !head.SameCommit(after) {
		t.Fatalf("probe changed the durable execution tree or its writer: found=%t, error=%v", found, err)
	}
}

func TestInteractionExecutorTerminatesWithoutReplayingUnknownEffect(t *testing.T) {
	workspace := t.TempDir()
	var calls int
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		calls++
		return interactionUsageTextResponse("externally completed", 2, 1), nil
	})
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		UnknownEffectPollInterval: durationPointer(5 * time.Millisecond),
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	session, err := executor.session(ref)
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := observeTestInteraction(t, executor, context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	unknownReady := make(chan struct{})
	go func() {
		for event := range sequence {

			if commit, authoritative := event.Payload.(runs.ExecutionFactCommit); authoritative {
				var commitErr error
				if _, completed := commit.Fact().(runs.ModelCallCompleted); completed {
					commitErr = errors.New("final projection is indeterminate")
				}
				event.Payload = commit.Fact()
				commit.Complete(commitErr)
			}
			if _, unknown := event.Payload.(runs.SegmentEnded); unknown {
				close(unknownReady)
				return
			}
		}
	}()
	if err := executor.BeginRoot(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	select {
	case <-unknownReady:
	case <-time.After(waitBudget(time.Second)):
		t.Fatal("unknown Effect was not observed")
	}
	process := session.state.processHandle()
	if process == nil {
		t.Fatal("unknown Interaction has no Process")
	}
	result, err := process.Await(t.Context())
	if err != nil || len(result.Termination().UnresolvedEffectIDs()) == 0 {
		t.Fatalf("terminal evidence missing: %v", err)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want no replay after terminal evidence collection", calls)
	}
}

func TestInteractionExecutorAppliesSteerAtNextModelBoundary(t *testing.T) {
	model := &runtimeSteerModel{started: make(chan struct{}), release: make(chan struct{})}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{})
	ref, err := executor.StageRoot(t.Context(), interactionTestStart())
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := observeTestInteraction(t, executor, context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	events := collectInteractionEvents(sequence)
	if err := executor.BeginRoot(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	<-model.started
	firstID, err := executor.SubmitSteer(t.Context(), ref, []transcript.ContentBlock{{
		Kind: transcript.TextContent, Text: "add evidence",
	}})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := executor.SubmitSteer(t.Context(), ref, []transcript.ContentBlock{{
		Kind: transcript.TextContent, Text: "add evidence",
	}})
	if err != nil {
		t.Fatal(err)
	}
	close(model.release)
	observed := <-events
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	steers := payloadsOf[runs.SteerMessagesApplied](observed)
	if len(steers) != 1 || len(steers[0].Messages) != 2 ||
		len(steers[0].Messages[0].Content) != 1 || steers[0].Messages[0].Content[0].Text != "add evidence" ||
		len(steers[0].Messages[1].Content) != 1 || steers[0].Messages[1].Content[0].Text != "add evidence" {
		t.Fatalf("steer projections = %#v", steers)
	}
	if firstID == secondID || steers[0].Messages[0].ItemID != firstID || steers[0].Messages[1].ItemID != secondID {
		t.Fatalf("admitted identities %q/%q differ from applied facts: %#v", firstID, secondID, steers)
	}
	steerIndex, secondModelIndex := -1, -1
	modelStarts := 0
	for index, event := range observed {
		switch event.Payload.(type) {
		case runs.SteerMessagesApplied:
			steerIndex = index
		case runs.ModelCallStarted:
			modelStarts++
			if modelStarts == 2 {
				secondModelIndex = index
			}
		}
	}
	if steerIndex < 0 || secondModelIndex < 0 || steerIndex >= secondModelIndex {
		t.Fatalf("event order steer=%d second model=%d: %#v", steerIndex, secondModelIndex, observed)
	}
}

func TestInteractionExecutorDoesNotCallNextModelWhenAppliedSteerCommitFails(t *testing.T) {
	model := &runtimeSteerModel{started: make(chan struct{}), release: make(chan struct{})}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{})
	ref, err := executor.StageRoot(t.Context(), interactionTestStart())
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := observeTestInteraction(t, executor, context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	eventsReady := make(chan []runs.ExecutorEvent, 1)
	go func() {
		var events []runs.ExecutorEvent
		for event := range sequence {

			if commit, authoritative := event.Payload.(runs.ExecutionFactCommit); authoritative {
				commitErr := error(nil)
				if _, applied := commit.Fact().(runs.SteerMessagesApplied); applied {
					commitErr = errors.New("steer store unavailable")
				}
				commit.Complete(commitErr)
				event.Payload = commit.Fact()
			}
			events = append(events, event)
		}
		eventsReady <- events
	}()
	if err := executor.BeginRoot(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	<-model.started
	if _, err := executor.SubmitSteer(t.Context(), ref, []transcript.ContentBlock{{
		Kind: transcript.TextContent, Text: "do not lose this",
	}}); err != nil {
		t.Fatal(err)
	}
	close(model.release)
	events := <-eventsReady
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	model.mu.Lock()
	calls := model.calls
	model.mu.Unlock()
	if calls != 1 {
		t.Fatalf("model calls = %d, want no next turn after rejected steer commit", calls)
	}
	if len(unresolvedTerminals(events)) != 0 {
		t.Fatalf("pre-call steer failure became unknown: %#v", events)
	}
	assertInternalProjectionTerminal(t, events)
}

type runtimeSteerModel struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (r *runtimeSteerModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	if call == 1 {
		if len(request.Messages) != 1 {
			return nil, errors.New("steer reached the in-flight model request")
		}
		close(r.started)
		<-r.release
		return interactionUsageTextResponse("draft", 1, 1), nil
	}
	if len(request.Messages) != 4 || request.Messages[1].Text() != "draft" ||
		request.Messages[2].Role != chat.RoleUser || request.Messages[2].Text() != "add evidence" ||
		request.Messages[3].Role != chat.RoleUser || request.Messages[3].Text() != "then shorten it" {
		return nil, errors.New("steer was not visible at the next model boundary")
	}
	return interactionUsageTextResponse("revised", 1, 1), nil
}

func captureInteractionQuestionCheckpoint(t *testing.T, workspace string) runs.ExecutorCheckpoint {
	t.Helper()
	question := newQuestionCheckpointTool(t)
	executor := newObservedTestInteractionExecutor(t, &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "ask_call", Name: "ask", Arguments: `{}`}, 1, 1),
	}}, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "ask")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{question}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Question}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	_, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error {
		return executor.BeginRoot(t.Context(), ref)
	})
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	return barrier.Checkpoint()
}

func newQuestionCheckpointTool(t *testing.T) toolcontract.Tool {
	t.Helper()
	question, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "ask", Description: "Ask before completing.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		_, err := runinput.Require(ctx, "question.ask", runs.Interrupt{
			Kind: interrupt.Question,
			Question: &runs.QuestionPrompt{
				ToolName: "ask", Arguments: `{}`,
				Fields: []runs.QuestionFieldSpec{{Prompt: "Which value?"}},
			},
		})
		return "", err
	})
	if err != nil {
		t.Fatal(err)
	}
	return question
}

func observeInteractionUntilWaiting(
	t *testing.T,
	executor *InteractionExecutor,
	ref runs.ExecutorRef,
	begin func() error,
) ([]runs.ExecutorEvent, runs.TreeInterrupted) {
	t.Helper()
	sequence, err := observeTestInteraction(t, executor, context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		events  []runs.ExecutorEvent
		barrier runs.TreeInterrupted
	}
	ready := make(chan result, 1)
	go func() {
		var value result
		sequence(func(event runs.ExecutorEvent) bool {

			if commit, authoritative := event.Payload.(runs.ExecutionFactCommit); authoritative {
				commit.Complete(nil)
				event.Payload = commit.Fact()
			}
			value.events = append(value.events, event)
			if barrier, waiting := event.Payload.(runs.TreeInterrupted); waiting {
				value.barrier = barrier
				return false
			}
			return true
		})
		ready <- value
	}()
	if err := begin(); err != nil {
		t.Fatal(err)
	}
	value := <-ready
	if len(value.barrier.Interruptions()) != 1 {
		t.Fatalf("waiting barrier = %#v", value.barrier)
	}
	return value.events, value.barrier
}

func collectInteractionEvents(sequence func(func(runs.ExecutorEvent) bool)) <-chan []runs.ExecutorEvent {
	ready := make(chan []runs.ExecutorEvent, 1)
	go func() {
		var events []runs.ExecutorEvent
		sequence(func(event runs.ExecutorEvent) bool {

			if commit, authoritative := event.Payload.(runs.ExecutionFactCommit); authoritative {
				commit.Complete(nil)
				event.Payload = commit.Fact()
			}
			events = append(events, event)
			return true
		})
		ready <- events
	}()
	return ready
}

// TestUnresumableWaitingExecutionReportsWhy pins the operator's half of a
// refused probe. Refusing recovers the whole waiting tree as lost — the user
// watches a parked Run disappear — and the conditions that produce it are not
// equally expected: a checkpoint from another build is routine after an
// upgrade, while state this build wrote and cannot read is a defect.
func TestUnresumableWaitingExecutionReportsWhy(t *testing.T) {
	var diagnostics bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&diagnostics, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	workspace := t.TempDir()
	checkpoint := captureInteractionQuestionCheckpoint(t, workspace)
	foreign := checkpoint.Clone()
	foreign.BuildID = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(
		func(context.Context, *chat.Request) (*chat.Response, error) {
			return nil, errors.New("model must not be called while probing")
		}), InteractionExecutorConfig{})
	continuation := rootInteractionWaitingContinuation(foreign, workspace, "exec_probe", run.Capabilities{})

	resumption, err := executor.CanResumeWaitingExecution(t.Context(), continuation)

	resumable := resumption.Resumable()
	if err != nil || resumable {
		t.Fatalf("CanResumeWaitingExecution = %t, %v, want false with no error", resumable, err)
	}
	logged := diagnostics.String()
	if !strings.Contains(logged, "not resumable") {
		t.Fatalf("diagnostics = %q, want the refusal reported", logged)
	}
	if !strings.Contains(logged, "another build") {
		t.Fatalf("diagnostics = %q, want the reason that separates an upgrade from a defect", logged)
	}
}
