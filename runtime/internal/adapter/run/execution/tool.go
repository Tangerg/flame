package execution

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/telemetry"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type observedInteractionTool struct {
	ref               tool.Ref
	receiptRelease    *framevalue.Digest
	sourceFingerprint framevalue.Digest
	inner             toolcontract.Tool
	binding           toolcontract.Binding
	session           *interactionSession
	interpreter       interactionToolInterpreter
	presenter         interactionToolPresenter
	authorizer        interactionToolAuthorizer
	hooks             interactionToolHooks
	offloadPolicy     toolResultOffloadPolicy
	start             runs.RootExecutionStart
	concurrent        func(toolcontract.Invocation) (string, bool)
}

func (o *observedInteractionTool) Definition() toolcontract.Definition {
	return o.binding.Contract().Definition()
}

func (o *observedInteractionTool) InputValidator() func([]byte) error {
	contract := o.binding.Contract()
	return func(arguments []byte) error {
		_, err := contract.Prepare(toolcontract.Request{Name: contract.Name(), Arguments: string(arguments)}, toolcontract.Admission{})
		return err
	}
}

func (o *observedInteractionTool) Unwrap() toolcontract.Tool { return o.inner }

func (o *observedInteractionTool) ConcurrencyPolicy() func(toolcontract.Invocation) (string, bool) {
	return o.concurrent
}

func (o *observedInteractionTool) Call(ctx context.Context, bound toolcontract.Invocation) (result toolcontract.Result, err error) {
	if err := o.session.awaitDispatchSegment(ctx); err != nil {
		return toolcontract.Result{}, err
	}
	invocation, reference, arguments, err := o.attributedInvocation(ctx, bound)
	if err != nil {
		return toolcontract.Result{}, err
	}
	defer func() { o.recordUnknownEffect(invocation, err) }()
	call := invocation.ToolCall()
	identity, err := logicalToolCallID(reference.process, reference.sequence, reference.index, call.ID, call.Name)
	if err != nil {
		return toolcontract.Result{}, err
	}
	callID := identity.String()
	member, hasCaller := o.session.toolCallMember(invocation.ProcessID())
	if !hasCaller {
		return toolcontract.Result{}, errors.New("execution: Tool call has no calling Interaction member")
	}
	ctx = runExecutionContext(ctx, o.session.scope, o.start)
	ctx = o.session.modelCallContext(ctx, member, callID)
	ctx = operation.Adopt(ctx, callID, extension.Model, member.MemberID)
	ctx = runinput.WithCapabilities(ctx, o.start.InterruptTypes)
	preparedInput, continuation, err := restoreToolInput(bound)
	if err != nil {
		return toolcontract.Result{}, err
	}
	prepared, err := o.prepare(ctx, bound, callID, call.Name, arguments, preparedInput)
	if err != nil {
		return toolcontract.Result{}, err
	}
	if prepared.input != nil {
		return toolcontract.Result{InputRequest: prepared.input}, nil
	}
	arguments = prepared.arguments

	start := runs.ToolCallStarted{
		CallID: callID, ModelCallSequence: reference.sequence,
		ToolCallIndex: reference.index, SourceCallID: call.ID, ToolName: call.Name, Source: o.ref,
		Input: transcript.AdmittedInput(arguments), SafetyClass: o.interpreter.SafetyClass(o.ref),
	}
	if err := o.commitStart(ctx, invocation, member, start, prepared.runs()); err != nil {
		return toolcontract.Result{}, err
	}
	var output toolcontract.Output
	callErr := prepared.failure
	var mutatedPaths []string
	if prepared.runs() {
		var ran toolcontract.Result
		ran, mutatedPaths, callErr = o.run(ctx, bound, call, arguments, continuation)
		if callErr == nil && ran.InputRequest != nil {
			return toolcontract.Result{InputRequest: ran.InputRequest}, nil
		}
		output = ran.Output
	}
	projection, err := o.projectToolResult(start.ToolName, arguments, output, callErr, mutatedPaths)
	if err != nil {
		return toolcontract.Result{}, err
	}
	if err := o.settle(ctx, member, invocation.ProcessID(), start, arguments, projection, callErr); err != nil {
		return toolcontract.Result{}, err
	}
	return toolcontract.Result{Output: projection.output}, projection.outcome
}

func (o *observedInteractionTool) recordUnknownEffect(invocation interaction.ToolInvocation, err error) {
	if err == nil {
		return
	}
	if _, known := toolcontract.ClassifyFailure(err); known {
		return
	}
	o.session.effectFailures.record(invocation.EffectID(), err)
	o.session.lifetime.wakeUnknown()
}

func (o *observedInteractionTool) commitStart(ctx context.Context, invocation interaction.ToolInvocation, member runs.ExecutorMember, start runs.ToolCallStarted, captures bool) error {
	var started runs.ExecutionFact = start
	if o.receiptRelease != nil && captures {
		var err error
		started, err = o.capturePreparedTool(ctx, invocation, member, start)
		if err != nil {
			return err
		}
	}
	if err := o.session.commitFact(ctx, member, started); err != nil {
		return o.projectionFailure(fmt.Errorf("execution: commit Tool call start: %w", err))
	}
	return nil
}

func (o *observedInteractionTool) run(ctx context.Context, bound toolcontract.Invocation, call corechat.ToolCall, arguments tool.Arguments, continuation *toolcontract.Continuation) (toolcontract.Result, []string, error) {
	var mutatedPaths []string
	ctx = toolset.WithMutationRecorder(ctx, func(paths []string) {
		mutatedPaths = append(mutatedPaths, paths...)
	})
	result, err := o.invoke(ctx, corechat.ToolCall{
		ID: call.ID, Name: call.Name, Arguments: arguments.Canonical(),
	}, arguments, toolcontract.Admission{IdempotencyKey: bound.IdempotencyKey(), Continuation: continuation})
	return result, mutatedPaths, err
}

func (o *observedInteractionTool) settle(ctx context.Context, member runs.ExecutorMember, processID scoperun.ProcessID, start runs.ToolCallStarted, arguments tool.Arguments, projection projectedToolResult, callErr error) error {
	metadata := toolCallMetadata{
		toolCallCapture: toolCallCapture{MemberID: member.MemberID, ProcessID: processID, Start: start},
		presentation:    &projection.presentation,
	}
	if err := o.session.rememberToolMetadata(metadata); err != nil {
		return o.projectionFailure(err)
	}
	if projection.presentation.failure != nil && projection.presentation.failure.Kind == tool.FailureDenied {
		return nil
	}
	outcomeCtx, cancelOutcome := context.WithTimeout(context.WithoutCancel(ctx), auxiliaryOperationTimeout)
	o.projectToolOutcome(outcomeCtx, member, start.ToolName, callErr == nil)
	cancelOutcome()
	o.runAfterToolUseHook(ctx, start.CallID, start.ToolName, arguments, projection.output, callErr)
	return nil
}

func restoreToolInput(bound toolcontract.Invocation) (*runinput.Continuation, *toolcontract.Continuation, error) {
	continued, resumed, err := runinput.Restore(bound)
	if err != nil || !resumed {
		return nil, nil, err
	}
	var continuation *toolcontract.Continuation
	if _, question := continued.Interrupt.(runs.QuestionPrompt); question {
		restored, _ := bound.Continuation()
		continuation = &restored
	}
	return &continued, continuation, nil
}

func (o *observedInteractionTool) capturePreparedTool(ctx context.Context, invocation interaction.ToolInvocation, member runs.ExecutorMember, start runs.ToolCallStarted) (runs.ToolCallPrepared, error) {
	proof := toolReceiptEvidence{
		EffectID: invocation.EffectID(), Release: *o.receiptRelease, Authority: o.sourceFingerprint,
	}
	capture := toolCallCapture{MemberID: member.MemberID, ProcessID: invocation.ProcessID(), Start: start, Receipt: &proof}
	if err := o.session.rememberToolMetadata(toolCallMetadata{toolCallCapture: capture}); err != nil {
		return runs.ToolCallPrepared{}, o.projectionFailure(err)
	}
	tree, err := o.session.committedTree(ctx, o.session.rootID)
	if err != nil {
		return runs.ToolCallPrepared{}, o.projectionFailure(err)
	}
	checkpoint, err := o.session.executorCheckpoint(tree)
	if err != nil {
		return runs.ToolCallPrepared{}, o.projectionFailure(err)
	}
	server, _, _ := start.Source.MCP()
	installation, _ := server.Origin().Installation()
	calling, err := operation.Capture(ctx, framecall.Target{Scope: extension.ScopeInstallation, Reference: installation.String()})
	if err != nil {
		return runs.ToolCallPrepared{}, err
	}
	caller, err := toolset.CaptureToolCaller(o.inner, calling)
	if err != nil {
		return runs.ToolCallPrepared{}, err
	}
	return runs.ToolCallPrepared{Start: start, Context: checkpoint, Caller: caller}, nil
}

type projectedToolResult struct {
	output       toolcontract.Output
	presentation toolResultPresentation
	outcome      error
}

func (o *observedInteractionTool) projectToolResult(toolName string, arguments tool.Arguments, output toolcontract.Output, callErr error, mutatedPaths []string) (projectedToolResult, error) {
	failure, knownFailure := toolcontract.ClassifyFailure(callErr)
	if callErr != nil {
		if !knownFailure {
			return projectedToolResult{}, callErr
		}
		output = failure.Output()
	}
	if err := output.Validate(); err != nil {
		return projectedToolResult{}, err
	}
	output = modeladapter.WithoutToolResultReferences(output)
	modelOutput := output
	presentation := toolResultPresentation{mutatedPaths: normalizeMutationPaths(mutatedPaths)}
	if failure == nil || failure.Kind() == toolcontract.FailureKindFailed {
		var err error
		modelOutput, presentation.offload, err = o.offloadPolicy.project(toolName, output)
		if err != nil {
			return projectedToolResult{}, fmt.Errorf("execution: project offloaded Tool result: %w", err)
		}
	}
	structuredOutput := modelOutput
	if presentation.offload != nil && presentation.offload.Ref.ModelOnly {
		structuredOutput = output
	}
	parsed, present, err := runtimeToolResult(structuredOutput)
	if err != nil {
		return projectedToolResult{}, err
	}
	if present {
		parsed, presentation.outputText = o.presenter.Present(o.ref, arguments, parsed)
		presentation.result = &parsed
	}
	var outcome error
	if failure != nil {
		presentation.failure = &tool.Failure{Kind: tool.FailureExecution, Detail: run.Diagnostic(callErr)}
		if failure.Kind() == toolcontract.FailureKindRejected {
			detail, _ := output.Text()
			presentation.failure = &tool.Failure{Kind: tool.FailureDenied, Detail: detail}
		}
		// Scope settles failures from Failure.Output instead of Result.Output.
		// Both must receive this projection so external metadata has no bypass.
		projected, err := toolcontract.NewFailure(toolcontract.FailureConfig{
			Kind: failure.Kind(), Output: modelOutput, Cause: callErr,
		})
		if err != nil {
			return projectedToolResult{}, err
		}
		outcome = projected
	}
	return projectedToolResult{output: modelOutput, presentation: presentation, outcome: outcome}, nil
}

func (o *observedInteractionTool) invoke(
	ctx context.Context,
	call corechat.ToolCall,
	arguments tool.Arguments,
	admission toolcontract.Admission,
) (toolcontract.Result, error) {
	bound, err := o.prepareInvocation(call, arguments, admission)
	if err != nil {
		return toolcontract.Result{}, err
	}
	return o.binding.Call(ctx, bound)
}

func (o *observedInteractionTool) prepareInvocation(call corechat.ToolCall, arguments tool.Arguments, admission toolcontract.Admission) (toolcontract.Invocation, error) {
	bound, err := o.binding.Contract().Prepare(call.Request(), admission)
	if err != nil {
		cause := fmt.Errorf("execution: prepare Tool %q invocation: %w", call.Name, err)
		failure, failureErr := toolcontract.NewFailure(toolcontract.FailureConfig{
			Kind: toolcontract.FailureKindFailed, Cause: cause,
			Output: toolcontract.NewTextOutput("invalid effective arguments: " + run.Diagnostic(cause)),
		})
		if failureErr != nil {
			return toolcontract.Invocation{}, failureErr
		}
		return toolcontract.Invocation{}, failure
	}
	if o.interpreter.UsesStandardPolicy(o.ref) {
		if err := o.validateApprovalSubject(call.Name, arguments); err != nil {
			return toolcontract.Invocation{}, err
		}
	}
	return bound, nil
}

func (o *observedInteractionTool) projectionFailure(cause error) error {
	o.session.lifetime.wakeUnknown()
	return hostFailure(cause)
}

func (o *observedInteractionTool) attributedInvocation(
	ctx context.Context,
	bound toolcontract.Invocation,
) (interaction.ToolInvocation, toolCallSource, tool.Arguments, error) {
	invocation, reference, err := o.session.toolSource(ctx, bound)
	if err != nil {
		return interaction.ToolInvocation{}, toolCallSource{}, tool.Arguments{}, err
	}
	call := invocation.ToolCall()
	if call.Name != o.Definition().Name {
		return interaction.ToolInvocation{}, toolCallSource{}, tool.Arguments{}, errors.New("execution: Tool invocation differs from its executable")
	}
	arguments, err := tool.ParseArguments(string(bound.Arguments()))
	if err != nil {
		return interaction.ToolInvocation{}, toolCallSource{}, tool.Arguments{}, err
	}
	return invocation, reference, arguments, nil
}

func (o *observedInteractionTool) projectToolOutcome(
	ctx context.Context,
	member runs.ExecutorMember,
	name string,
	succeeded bool,
) {
	projected, err := o.interpreter.ProjectOutcome(ctx, o.start.SessionID, o.ref, succeeded)
	if err != nil {
		slog.ErrorContext(ctx, "execution: project tool outcome failed",
			telemetry.SessionID(o.start.SessionID), telemetry.ToolName(name), telemetry.Error(err),
		)
		return
	}
	if projected != nil {
		o.session.lifetime.send(runs.ExecutorEvent{Member: member, Payload: projected})
	}
}

func (o *observedInteractionTool) runAfterToolUseHook(
	ctx context.Context,
	callID string,
	name string,
	arguments tool.Arguments,
	output toolcontract.Output,
	callErr error,
) {
	hookCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), auxiliaryOperationTimeout)
	defer cancel()
	result, err := hookToolOutput(output)
	if err == nil {
		err = o.hooks.AfterToolUse(hookCtx, InteractionToolHookInput{
			SessionID: o.start.SessionID, CWD: o.start.CWD, WorkspaceCWD: o.start.WorkspaceCWD,
			ToolName: name, Arguments: arguments, Result: result, CallError: callErr,
		})
	}
	if err != nil {
		slog.ErrorContext(hookCtx, "execution: post-tool hook failed",
			telemetry.SessionID(o.start.SessionID), telemetry.ToolName(name), telemetry.ToolCallID(callID), telemetry.Error(err),
		)
	}
}

func runtimeToolResult(output toolcontract.Output) (tool.Result, bool, error) {
	if len(output.Content) == 0 && len(output.Details) == 0 {
		return tool.Result{}, false, nil
	}
	if len(output.Details) > 0 {
		parsed, err := tool.ParseResult(output.Details)
		return parsed, true, err
	}
	if text, textual := output.Text(); textual {
		if parsed, err := tool.ParseResult([]byte(text)); err == nil {
			return parsed, true, nil
		}
		result, err := tool.NewResult(text)
		return result, true, err
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return tool.Result{}, false, err
	}
	parsed, err := tool.ParseResult(encoded)
	return parsed, true, err
}

func hookToolOutput(output toolcontract.Output) (string, error) {
	if text, textual := output.Text(); textual {
		return text, nil
	}
	encoded, err := json.Marshal(output)
	return string(encoded), err
}

func normalizeMutationPaths(paths []string) []string {
	paths = slices.DeleteFunc(slices.Clone(paths), func(path string) bool { return path == "" })
	slices.Sort(paths)
	return slices.Compact(paths)
}

func wrapInteractionTools(
	ctx context.Context,
	manifest toolset.Manifest,
	session *interactionSession,
	config InteractionExecutorConfig,
	offloadPolicy toolResultOffloadPolicy,
	start runs.RootExecutionStart,
) (visible []toolcontract.Tool, deferred []toolcontract.Tool, contracts map[tool.Ref]toolcontract.Contract, owners map[tool.Ref]*observedInteractionTool, err error) {
	wrapping := toolWrapping{
		session: session, config: config, offloadPolicy: offloadPolicy, start: start,
		contracts: make(map[tool.Ref]toolcontract.Contract), owners: make(map[tool.Ref]*observedInteractionTool),
	}
	visible, err = wrapping.wrapAll(ctx, manifest.Visible)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	deferred, err = wrapping.wrapAll(ctx, manifest.Deferred)
	return visible, deferred, wrapping.contracts, wrapping.owners, err
}

type toolWrapping struct {
	session       *interactionSession
	config        InteractionExecutorConfig
	offloadPolicy toolResultOffloadPolicy
	start         runs.RootExecutionStart
	contracts     map[tool.Ref]toolcontract.Contract
	owners        map[tool.Ref]*observedInteractionTool
}

func (w toolWrapping) wrapAll(ctx context.Context, values []toolcontract.Tool) ([]toolcontract.Tool, error) {
	wrapped := make([]toolcontract.Tool, len(values))
	for index, executable := range values {
		observed, err := w.observe(ctx, executable)
		if err != nil {
			return nil, fmt.Errorf("execution: Interaction Tool[%d]: %w", index, err)
		}
		w.contracts[observed.ref] = observed.binding.Contract()
		w.owners[observed.ref] = observed
		wrapped[index] = observed
	}
	return wrapped, nil
}

func (w toolWrapping) observe(ctx context.Context, executable toolcontract.Tool) (*observedInteractionTool, error) {
	binding, err := toolcontract.Bind(executable)
	if err != nil {
		return nil, err
	}
	ref, err := toolset.Identify(executable)
	if err != nil {
		return nil, err
	}
	fingerprint, err := toolset.SourceFingerprint(executable, ref)
	if err != nil {
		return nil, err
	}
	if class := w.config.ToolInterpreter.SafetyClass(ref); !class.Valid() {
		return nil, fmt.Errorf("execution: Interaction Tool %q has invalid safety class %q", binding.Contract().Definition().Name, class)
	}
	observed := &observedInteractionTool{
		ref: ref, sourceFingerprint: fingerprint, inner: executable, binding: binding, session: w.session, interpreter: w.config.ToolInterpreter,
		presenter: w.config.ToolPresenter, authorizer: w.config.ToolAuthorizer,
		hooks: w.config.ToolHooks, offloadPolicy: w.offloadPolicy,
		start: w.start,
	}
	if err := observed.captureReceiptSource(ctx, w.config.OriginalToolResults); err != nil {
		return nil, err
	}
	// Proposals, policy prompts and approval interrupts can pause or reshape a
	// call, so they rule out concurrency.
	if len(w.session.toolProposals) == 0 && !w.config.ToolInterpreter.UsesStandardPolicy(ref) &&
		!slices.Contains(w.start.InterruptTypes, run.InterruptApproval) {
		if concurrent, found := toolcontract.As[interaction.ConcurrentTool](executable); found {
			observed.concurrent = keyFreeConcurrency(concurrent.ConcurrencyPolicy())
		}
	}
	return observed, nil
}

// Lifecycle hooks are live configuration that may rewrite arguments after Scope
// classifies a call, so a resource key derived from the original arguments
// cannot be trusted; only key-free classifications stay concurrent.
func keyFreeConcurrency(classify func(toolcontract.Invocation) (string, bool)) func(toolcontract.Invocation) (string, bool) {
	if classify == nil {
		return nil
	}
	return func(invocation toolcontract.Invocation) (string, bool) {
		key, concurrent := classify(invocation)
		return "", concurrent && key == ""
	}
}

func (o *observedInteractionTool) captureReceiptSource(ctx context.Context, results originalToolResults) error {
	server, _, mcp := o.ref.MCP()
	if !mcp || results == nil {
		return nil
	}
	if _, installed := server.Origin().Installation(); !installed {
		return nil
	}
	dependency, found, err := toolset.InstallationDependency(o.inner)
	if err != nil || !found {
		return err
	}
	activation, err := toolset.InstallationActivation(o.inner)
	if err != nil {
		return err
	}
	owned, err := results.OwnsToolSource(ctx, o.ref, dependency.Digest, activation)
	if err != nil {
		return err
	}
	if owned {
		o.receiptRelease = new(dependency.Digest)
	}
	return nil
}
