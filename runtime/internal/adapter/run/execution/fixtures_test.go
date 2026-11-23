package execution

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/jsonschema"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

const interactionTestRunID = "run_root"

func interactionTestStart() runs.RootExecutionStart {
	return runs.RootExecutionStart{
		SessionID: testsupport.MustSessionID("session_1"), RunID: testsupport.MustRunID(interactionTestRunID), ModelSelection: testDefaultSelection(),
		WorkingContext: []chat.Message{chat.NewUserMessage(chat.NewTextPart("current question"))},
	}
}

func captureAcceptedTestBarrier(t *testing.T, executor *InteractionExecutor, ref runs.ExecutorRef, count int) runs.TreeInterrupted {
	t.Helper()
	session, err := executor.session(ref)
	if err != nil {
		t.Fatal(err)
	}
	session.lifetime.stopReconciling()
	sequence, err := observeTestInteraction(t, executor, t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan runs.TreeInterrupted, 1)
	go func() {
		var result runs.TreeInterrupted
		for event := range sequence {
			if commit, ok := event.Payload.(runs.ExecutionFactCommit); ok {
				commit.Complete(nil)
			}
			if barrier, ok := event.Payload.(runs.TreeInterrupted); ok {
				result = barrier
				break
			}
		}
		ready <- result
	}()
	if err := executor.BeginRoot(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		root := session.state.processHandle()
		if root != nil {
			head, err := session.committedTree(ctx, root.ID())
			if err == nil {
				inputs, err := session.pendingInputs(head)
				if err != nil {
					t.Fatal(err)
				}
				if len(inputs) == count {
					break
				}
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("Tool waits did not commit")
		}
	}
	tree, interruptions, found, err := session.captureHumanInputBarrier(ctx)
	if err != nil || !found || len(interruptions) != count {
		t.Fatalf("paused barrier=%d, %t, %v", len(interruptions), found, err)
	}
	checkpoint, err := session.executorCheckpoint(tree)
	if err != nil {
		t.Fatal(err)
	}
	barrier, err := runs.NewTreeInterrupted(checkpoint, interruptions)
	if err != nil {
		t.Fatal(err)
	}
	session.state.mu.Lock()
	session.state.holdBoundary(interactionBoundaryWaiting)
	session.state.waitingCheckpoint = checkpoint.Clone()
	session.state.mu.Unlock()
	session.lifetime.send(runs.ExecutorEvent{Member: session.executorMember(session.state.processHandle().ID()), Payload: barrier})
	select {
	case result := <-ready:
		return result
	case <-ctx.Done():
		t.Fatal("barrier observer did not detach")
	}
	return runs.TreeInterrupted{}
}

func payloadsOf[T any](events []runs.ExecutorEvent) []T {
	return slices.Collect(func(yield func(T) bool) {
		for _, event := range events {
			if batch, ok := event.Payload.(runs.ToolResultsCommitted); ok {
				for _, result := range batch.Results {
					if payload, matches := any(result).(T); matches && !yield(payload) {
						return
					}
				}
			}
			if payload, ok := event.Payload.(T); ok && !yield(payload) {
				return
			}
		}
	})
}

func interactionChatResolver(
	model chat.Model,
	counter modeladapter.InputTokenCounter,
) InteractionChatResolver {
	return interactionChatResolverFunc(func(_ context.Context, selection flamemodel.Selection) (modeladapter.ResolvedChat, error) {
		return testsupport.OfflineChat(model, counter, testsupport.ModelFacts(selection), testsupport.Unpriced())
	})
}

type interactionChatResolverFunc func(context.Context, flamemodel.Selection) (modeladapter.ResolvedChat, error)

func (i interactionChatResolverFunc) ResolveChat(
	ctx context.Context,
	selection flamemodel.Selection,
) (modeladapter.ResolvedChat, error) {
	return i(ctx, selection)
}

func (r interactionChatResolverFunc) RestoreChat(ctx context.Context, selection flamemodel.Selection, _ *modeladapter.ProviderCapture) (modeladapter.ResolvedChat, error) {
	return r.ResolveChat(ctx, selection)
}

const interactionTestBuildID = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func durationPointer(value time.Duration) *time.Duration {
	return &value
}

func interactionTextResponse(text string) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	return &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}}
}

func newQuestionTool(t *testing.T, name, description string, fields []transcript.QuestionField, render func(run.InterruptResolution) (string, error)) toolcontract.Tool {
	t.Helper()
	schema, err := jsonschema.For[struct{}]()
	if err != nil {
		t.Fatal(err)
	}
	definition := toolcontract.Definition{Name: name, Description: description, InputSchema: schema.JSON()}
	if err := definition.Validate(); err != nil {
		t.Fatal(err)
	}
	return &questionTool{
		definition: definition,
		prompt:     runs.QuestionPrompt{ToolName: name, Arguments: `{}`, Fields: fields},
		render:     render,
	}
}

type observationScriptModel struct {
	mu        sync.Mutex
	responses []*chat.Response
}

func (o *observationScriptModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.responses) == 0 {
		return nil, errors.New("unexpected model call")
	}
	response := o.responses[0]
	o.responses = o.responses[1:]
	return response.Clone(), nil
}

func interactionToolBatchResponse(calls []chat.ToolCall, inputTokens, outputTokens int64) *chat.Response {
	parts := make([]chat.Part, len(calls))
	for index, call := range calls {
		parts[index] = chat.NewToolCallPart(call)
	}
	message := chat.NewAssistantMessage(parts...)
	return &chat.Response{
		Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls},
		Metadata: &chat.ResponseMetadata{
			Model: "test-model", Usage: &chat.Usage{InputTokens: inputTokens, OutputTokens: outputTokens},
		},
	}
}

func interactionUsageTextResponse(text string, inputTokens, outputTokens int64) *chat.Response {
	response := interactionTextResponse(text)
	response.Metadata = &chat.ResponseMetadata{
		Model: "test-model", Usage: &chat.Usage{InputTokens: inputTokens, OutputTokens: outputTokens},
	}
	return response
}

func newObservedTestInteractionExecutor(
	t *testing.T,
	model chat.Model,
	extra InteractionExecutorConfig,
) *InteractionExecutor {
	t.Helper()
	var counter modeladapter.InputTokenCounter
	if modelCounter, ok := model.(modeladapter.InputTokenCounter); ok {
		counter = modelCounter
	}
	extra.ChatResolver = interactionChatResolver(model, counter)
	extra.Lifetime = t.Context()
	extra.BuildID = interactionTestBuildID
	extra.UnknownEffectPollInterval = durationPointer(5 * time.Millisecond)
	executor, err := newTestConfiguredInteractionExecutor(t, extra)
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

type staticInteractionTools struct {
	manifest   toolset.Manifest
	identities []domaintool.Ref
}

func (s staticInteractionTools) Manifest(_ context.Context, _ domaintool.Group, retained *toolset.Retained) (toolset.Manifest, error) {
	manifest, err := identifyTestManifest(s.manifest, s.identities...)
	if err != nil || retained == nil {
		return manifest, err
	}
	for _, group := range []*[]toolcontract.Tool{&manifest.Visible, &manifest.Deferred} {
		selected := (*group)[:0]
		for _, executable := range *group {
			ref, err := toolset.Identify(executable)
			if err != nil {
				return toolset.Manifest{}, err
			}
			if _, _, mcp := ref.MCP(); mcp && !slices.Contains(retained.MCP, ref) {
				continue
			}
			selected = append(selected, executable)
		}
		*group = selected
	}
	return manifest, nil
}

type concurrentInteractionTool struct{ toolcontract.Tool }

func (concurrentInteractionTool) ConcurrencyPolicy() func(toolcontract.Invocation) (string, bool) {
	return func(toolcontract.Invocation) (string, bool) { return "", true }
}

type immutableToolInterpreter struct{ testInteractionToolInterpreter }

func (immutableToolInterpreter) UsesStandardPolicy(domaintool.Ref) bool { return false }

type testInteractionToolInterpreter struct{}

func (testInteractionToolInterpreter) SafetyClass(domaintool.Ref) domaintool.SafetyClass {
	return domaintool.SafetyClassSafe
}

func (testInteractionToolInterpreter) UsesStandardPolicy(domaintool.Ref) bool { return true }

func (testInteractionToolInterpreter) ApprovalSubject(domaintool.Ref, domaintool.Arguments) (string, error) {
	return "", nil
}

func (testInteractionToolInterpreter) ShellCommand(domaintool.Ref, string) string { return "" }

func (testInteractionToolInterpreter) ProjectOutcome(context.Context, identity.SessionID, domaintool.Ref, bool) (runs.ExecutionFact, error) {
	return nil, nil
}

type allowInteractionTools struct{}

func (allowInteractionTools) AuthorizeTool(context.Context, ToolAuthorizationRequest) (ToolAuthorizationDecision, error) {
	return allowTool(), nil
}

func (allowInteractionTools) ResolveToolApproval(context.Context, ToolAuthorizationRequest, runs.ApprovalPrompt, run.InterruptResolution) (ToolAuthorizationDecision, error) {
	return allowTool(), nil
}

func rootInteractionExecutionContinuation(
	checkpoint runs.ExecutorCheckpoint,
	workspace string,
	executorID string,
	capabilities run.Capabilities,
) runs.ExecutionContinuation {
	return runs.ExecutionContinuation{
		SessionID: checkpoint.SessionID, ExecutorID: executorID, RootRunID: testsupport.MustRunID(interactionTestRunID),
		Members: []runs.ExecutionMember{{
			RunID: testsupport.MustRunID(interactionTestRunID), MemberID: checkpoint.RootMemberID,
			ModelSelection: interactionTestStart().ModelSelection,
		}},
		Checkpoint: checkpoint, Capabilities: capabilities, Workspace: workspace,
	}
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

type questionTool struct {
	definition toolcontract.Definition
	prompt     runs.Interrupt
	render     func(run.InterruptResolution) (string, error)
}

func (q *questionTool) Definition() toolcontract.Definition { return q.definition }

func (q *questionTool) Call(ctx context.Context, invocation toolcontract.Invocation) (toolcontract.Result, error) {
	resolution, request, err := runinput.Require(ctx, invocation, q.prompt)
	if err != nil {
		return toolcontract.Result{}, err
	}
	if request != nil {
		return toolcontract.Result{InputRequest: request}, nil
	}
	text, err := q.render(resolution)
	if err != nil {
		return toolcontract.Result{}, err
	}
	output := toolcontract.NewTextOutput(text)
	if err := output.Validate(); err != nil {
		return toolcontract.Result{}, err
	}
	return toolcontract.Result{Output: output}, nil
}

func identifyTestManifest(manifest toolset.Manifest, identities ...domaintool.Ref) (toolset.Manifest, error) {
	manifest = manifest.Clone()
	for _, group := range [][]toolcontract.Tool{manifest.Visible, manifest.Deferred} {
		for index, executable := range group {
			if _, err := toolcontract.Bind(executable); err != nil {
				return toolset.Manifest{}, err
			}
			if _, err := toolset.Identify(executable); err == nil {
				continue
			}
			var ref domaintool.Ref
			for _, candidate := range identities {
				if candidate.ModelName() == executable.Definition().Name {
					ref = candidate
					break
				}
			}
			if err := ref.Validate(); err != nil {
				return toolset.Manifest{}, fmt.Errorf("test manifest has no explicit identity for %s: %w", executable.Definition().Name, err)
			}
			identified, err := toolset.WithIdentity(executable, ref, testsupport.ToolFingerprint(ref))
			if err != nil {
				return toolset.Manifest{}, err
			}
			group[index] = identified
		}
	}
	return manifest, nil
}
