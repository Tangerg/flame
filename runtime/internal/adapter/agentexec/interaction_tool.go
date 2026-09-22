package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/adapter/agentexec/interactioninput"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type observedInteractionTool struct {
	inner         toolcontract.Tool
	binding       toolcontract.Binding
	session       *interactionSession
	interpreter   InteractionToolInterpreter
	presenter     InteractionToolPresenter
	authorizer    InteractionToolAuthorizer
	hooks         InteractionToolHooks
	offloader     toolResultOffloader
	offloadPolicy toolResultOffloadPolicy
	start         runs.RootExecutionStart
	concurrent    interaction.ConcurrentTool
}

func (o *observedInteractionTool) Definition() corechat.ToolDefinition {
	return o.binding.Contract().Definition()
}

func (o *observedInteractionTool) Unwrap() toolcontract.Tool { return o.inner }

// Arguments rewritten after admission cannot retain an inner resource key.
// Scope schedules these calls exclusively; immutable calls keep the declaration.
func (o *observedInteractionTool) ConcurrencyPolicy() func(toolcontract.Invocation) (string, bool) {
	if o.concurrent == nil {
		return nil
	}
	return o.concurrent.ConcurrencyPolicy()
}

func (o *observedInteractionTool) Call(ctx context.Context, bound toolcontract.Invocation) (returned corechat.ToolOutput, returnedErr error) {
	invocation, arguments, callID, err := o.attributedInvocation(ctx, bound)
	if err != nil {
		return corechat.ToolOutput{}, err
	}
	defer func() {
		if returnedErr == nil {
			return
		}
		if !errors.Is(returnedErr, interaction.ErrHostFailure) {
			if _, known := errors.AsType[*toolcontract.Failure](returnedErr); known {
				return
			}
			if errors.Is(returnedErr, interaction.ErrToolInputRequired) &&
				!errors.Is(returnedErr, context.Canceled) && !errors.Is(returnedErr, context.DeadlineExceeded) {
				return
			}
		}
		o.session.effectFailures.record(invocation.EffectID(), returnedErr)
	}()
	call := invocation.ToolCall()
	member, hasCaller := o.session.toolCallMember(invocation.Relation())
	if !hasCaller {
		return corechat.ToolOutput{}, errors.New("agentexec: Tool call has no calling Interaction member")
	}
	ctx = runExecutionContext(ctx, o.session.scope, o.start)
	ctx = interactioninput.WithCapabilities(ctx, o.start.InterruptKinds)
	if err := o.session.awaitDispatchSegment(ctx); err != nil {
		return corechat.ToolOutput{}, err
	}
	effectiveArguments, denied, denialReason, prepareErr := o.prepare(ctx, callID, call.Name, arguments)
	if prepareErr != nil {
		if _, known := errors.AsType[*toolcontract.Failure](prepareErr); !known || errors.Is(prepareErr, interaction.ErrHostFailure) {
			return corechat.ToolOutput{}, prepareErr
		}
	}
	arguments = effectiveArguments

	rawArguments := arguments.Canonical()
	start := runs.ToolCallStarted{
		CallID: callID, ModelCallSequence: invocation.ModelCallSequence(),
		ToolCallIndex: invocation.ToolCallIndex(), SourceCallID: call.ID, ToolName: call.Name,
		Arguments: rawArguments, Activity: o.activity(call.Name, arguments),
		SafetyClass: o.interpreter.SafetyClass(call.Name),
	}
	if err := o.session.commitFact(ctx, member, start); err != nil {
		return corechat.ToolOutput{}, o.projectionFailure(fmt.Errorf("agentexec: commit Tool call start: %w", err))
	}
	o.session.accounting.recordToolCall()
	var output corechat.ToolOutput
	var callErr error
	var mutatedPaths []string
	if prepareErr != nil {
		callErr = prepareErr
	} else if denied {
		failure, err := toolcontract.NewFailure(toolcontract.FailureConfig{
			Kind: toolcontract.FailureKindRejected, Output: corechat.NewTextToolOutput(denialReason),
		})
		if err != nil {
			return corechat.ToolOutput{}, err
		}
		callErr = failure
	} else {
		ctx = toolset.WithToolAdvertiser(ctx, func(names ...string) error {
			return interaction.AdvertiseTools(ctx, names...)
		})
		ctx = toolset.WithMutationRecorder(ctx, func(paths []string) {
			mutatedPaths = append(mutatedPaths, paths...)
		})
		output, callErr = o.invoke(ctx, corechat.ToolCall{
			ID: call.ID, Name: call.Name, Arguments: rawArguments,
		}, arguments)
	}

	if errors.Is(callErr, interaction.ErrHostFailure) {
		return corechat.ToolOutput{}, callErr
	}
	var failure *toolcontract.Failure
	if callErr != nil {
		if !errors.As(callErr, &failure) {
			return corechat.ToolOutput{}, callErr
		}
		if err := failure.Validate(); err != nil {
			return corechat.ToolOutput{}, err
		}
		output = failure.Output()
	}
	// Offload and presentation transform values before Scope settles them. They
	// must never turn an invalid external output into an apparently valid result.
	if err := output.Validate(); err != nil {
		return corechat.ToolOutput{}, err
	}
	modelOutput, offload := o.offload(ctx, call.Name, output, callErr)
	metadata := toolResultMetadata{
		MemberID: member.MemberID, Start: start, Arguments: arguments.Canonical(),
		Offload: offload, MutatedPaths: normalizeMutationPaths(mutatedPaths),
	}
	parsed, present, err := runtimeToolResult(modelOutput)
	if err != nil {
		return corechat.ToolOutput{}, err
	}
	if present {
		if o.presenter != nil {
			parsed, metadata.OutputText = o.presenter.Present(call.Name, arguments, parsed)
		}
		metadata.Result = &parsed
	}
	if failure != nil {
		metadata.Failure = &tool.Failure{Kind: tool.FailureExecution, Detail: executorDiagnostic(callErr)}
		if failure.Kind() == toolcontract.FailureKindRejected {
			detail, _ := output.Text()
			metadata.Failure = &tool.Failure{Kind: tool.FailureDenied, Detail: detail}
		}
	}
	if err := o.session.rememberToolMetadata(metadata); err != nil {
		return corechat.ToolOutput{}, o.projectionFailure(err)
	}
	if failure == nil || failure.Kind() != toolcontract.FailureKindRejected {
		outcomeCtx, cancelOutcome := context.WithTimeout(
			context.WithoutCancel(ctx), auxiliaryOperationTimeout,
		)
		o.projectToolOutcome(outcomeCtx, member, call.Name, callErr == nil)
		cancelOutcome()
		o.runAfterToolUseHook(ctx, callID, call.Name, arguments, modelOutput, callErr)
	}
	return modelOutput, callErr
}

// invoke validates the effective arguments before entering the external Tool.
// This boundary can prove that an invalid edit never entered the executable.
func (o *observedInteractionTool) invoke(
	ctx context.Context,
	call corechat.ToolCall,
	arguments tool.Arguments,
) (corechat.ToolOutput, error) {
	bound, err := o.binding.Contract().Prepare(call)
	if err != nil {
		cause := fmt.Errorf("agentexec: prepare Tool %q invocation: %w", call.Name, err)
		failure, failureErr := toolcontract.NewFailure(toolcontract.FailureConfig{
			Kind: toolcontract.FailureKindFailed, Cause: cause,
			Output: corechat.NewTextToolOutput("invalid effective arguments: " + executorDiagnostic(cause)),
		})
		if failureErr != nil {
			return corechat.ToolOutput{}, failureErr
		}
		return corechat.ToolOutput{}, failure
	}
	if o.interpreter.UsesStandardPolicy(call.Name) {
		if _, err := o.approvalSubject(call.Name, arguments); err != nil {
			return corechat.ToolOutput{}, err
		}
	}
	return o.binding.Call(ctx, bound)
}

// projectionFailure refuses to produce a Tool result the Host could not record.
// Unlike a model call, a Tool has no definite host-failure settlement: its
// Effect stays unknown until the Host resolves it, so every refusal here leaves
// the Run for the unknown reporter to settle and wakes it directly.
func (o *observedInteractionTool) projectionFailure(cause error) error {
	o.session.lifetime.wakeUnknown()
	return interaction.HostFailure(cause)
}

func (o *observedInteractionTool) attributedInvocation(
	ctx context.Context,
	bound toolcontract.Invocation,
) (interaction.ToolInvocation, tool.Arguments, string, error) {
	invocation, ok := interaction.ToolInvocationFromContext(ctx)
	if !ok {
		return interaction.ToolInvocation{}, tool.Arguments{}, "", errors.New("agentexec: Tool call has no Interaction attribution")
	}
	call := invocation.ToolCall()
	if call.Name != o.Definition().Name {
		return interaction.ToolInvocation{}, tool.Arguments{}, "", errors.New("agentexec: Tool invocation differs from its bound executable")
	}
	arguments, err := tool.ParseArguments(string(bound.Arguments()))
	if err != nil {
		return interaction.ToolInvocation{}, tool.Arguments{}, "", fmt.Errorf("agentexec: parse Tool %q arguments: %w", call.Name, err)
	}
	callIdentity, err := toolInvocationID(invocation)
	if err != nil {
		return interaction.ToolInvocation{}, tool.Arguments{}, "", err
	}
	return invocation, arguments, callIdentity.String(), nil
}

func (o *observedInteractionTool) projectToolOutcome(
	ctx context.Context,
	member runs.ExecutorMember,
	name string,
	succeeded bool,
) {
	projected, err := o.interpreter.ProjectOutcome(ctx, o.start.SessionID, name, succeeded)
	if err != nil {
		slog.ErrorContext(ctx, "agentexec: project tool outcome failed",
			"session.id", o.start.SessionID, "tool.name", name, "error", err,
		)
		return
	}
	if projected != nil {
		// Tool outcome projection is a refetchable live hint (for example a Plan
		// snapshot). Scope separately owns publication of the canonical result;
		// losing this hint cannot change its execution outcome.
		o.session.lifetime.send(runs.ExecutorEvent{Member: member, Payload: projected})
	}
}

func (o *observedInteractionTool) runAfterToolUseHook(
	ctx context.Context,
	callID string,
	name string,
	arguments tool.Arguments,
	output corechat.ToolOutput,
	callErr error,
) {
	if o.hooks == nil {
		return
	}
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
		slog.ErrorContext(hookCtx, "agentexec: post-tool hook failed",
			"session.id", o.start.SessionID, "tool.name", name, "call.id", callID, "error", err,
		)
	}
}

func (o *observedInteractionTool) prepare(
	ctx context.Context,
	callID string,
	name string,
	arguments tool.Arguments,
) (tool.Arguments, bool, string, error) {
	continued, resumed, err := interactioninput.Restore(ctx)
	if err != nil {
		return tool.Arguments{}, false, "", err
	}
	if resumed {
		return o.resumePreparedTool(ctx, callID, name, continued)
	}
	forceApproval := false
	if o.hooks != nil {
		decision, beforeToolUseErr := o.hooks.BeforeToolUse(ctx, InteractionToolHookInput{
			SessionID: o.start.SessionID, CWD: o.start.CWD, WorkspaceCWD: o.start.WorkspaceCWD,
			ToolName: name, Arguments: arguments,
		})
		if beforeToolUseErr != nil {
			return tool.Arguments{}, false, "", interaction.HostFailure(fmt.Errorf("agentexec: run pre-Tool hook: %w", beforeToolUseErr))
		}
		if rewritten, ok := decision.EffectiveArguments(); ok {
			arguments = rewritten
		}
		if reason, denied := decision.Denied(); denied {
			return arguments, true, reason, nil
		}
		forceApproval = decision.RequiresApproval()
	}
	if !o.interpreter.UsesStandardPolicy(name) {
		if forceApproval {
			return arguments, true, "a lifecycle hook requires approval, but approval is unavailable", nil
		}
		return arguments, false, "", nil
	}
	request, err := o.authorizationRequest(callID, name, arguments, forceApproval)
	if err != nil {
		return arguments, false, "", err
	}
	decision, err := o.authorizer.AuthorizeTool(ctx, request)
	if err != nil {
		return tool.Arguments{}, false, "", interaction.HostFailure(fmt.Errorf("agentexec: authorize Tool %q: %w", name, err))
	}
	if rewritten, ok := decision.EffectiveArguments(); ok {
		arguments = rewritten
	}
	if reason, denied := decision.Denied(); denied {
		return arguments, true, reason, nil
	}
	if prompt, ok := decision.Approval(); ok {
		return o.requestToolApproval(ctx, request, prompt)
	}
	return arguments, false, "", nil
}

func (o *observedInteractionTool) approvalSubject(name string, arguments tool.Arguments) (string, error) {
	subject, err := o.interpreter.ApprovalSubject(name, arguments)
	if err != nil {
		cause := fmt.Errorf("agentexec: derive Tool %q approval subject: %w", name, err)
		if errors.Is(err, tool.ErrInvalidArguments) {
			failure, failureErr := toolcontract.NewFailure(toolcontract.FailureConfig{
				Kind: toolcontract.FailureKindFailed, Cause: cause,
				Output: corechat.NewTextToolOutput("invalid effective arguments: " + executorDiagnostic(cause)),
			})
			if failureErr != nil {
				return "", interaction.HostFailure(failureErr)
			}
			return "", failure
		}
		return "", interaction.HostFailure(cause)
	}
	return subject, nil
}

func (o *observedInteractionTool) authorizationRequest(
	callID string,
	name string,
	arguments tool.Arguments,
	requireApproval bool,
) (ToolAuthorizationRequest, error) {
	subject, err := o.approvalSubject(name, arguments)
	if err != nil {
		return ToolAuthorizationRequest{}, err
	}
	autoApproved := false
	if o.session.mcpToolAutoApproved != nil {
		ref, found, err := mcp.IdentifyTool(o.inner)
		if err != nil {
			return ToolAuthorizationRequest{}, interaction.HostFailure(fmt.Errorf("agentexec: resolve MCP Tool identity: %w", err))
		}
		if found {
			autoApproved = o.session.mcpToolAutoApproved(ref.Server.String(), ref.Tool.String())
		}
	}
	return ToolAuthorizationRequest{
		SessionID: o.start.SessionID, WorkspaceCWD: o.start.WorkspaceCWD,
		CallID: callID, ToolName: name, Arguments: arguments,
		SafetyClass:     o.interpreter.SafetyClass(name),
		ApprovalSubject: subject,
		FileMutation:    fileMutationScope(o.inner, arguments, o.start.CWD),
		ShellCommand:    o.interpreter.ShellCommand(name, arguments.Canonical()),
		AutoApproved:    autoApproved,
		RequireApproval: requireApproval,
	}, nil
}

func (o *observedInteractionTool) requestToolApproval(
	ctx context.Context,
	request ToolAuthorizationRequest,
	prompt runs.ApprovalPrompt,
) (tool.Arguments, bool, string, error) {
	if !slices.Contains(o.start.InterruptKinds, interrupt.Approval) {
		return request.Arguments, true, "approval input is unavailable for this Run", nil
	}
	if prompt.CallID == "" {
		prompt.CallID = request.CallID
	}
	pending := runs.Interrupt{Kind: interrupt.Approval, Approval: &prompt}
	if err := pending.Validate(); err != nil {
		return tool.Arguments{}, false, "", fmt.Errorf("agentexec: invalid Tool approval prompt: %w", err)
	}
	if prompt.CallID != request.CallID || prompt.ToolName != request.ToolName ||
		prompt.Arguments != request.Arguments.Canonical() || prompt.SafetyClass != request.SafetyClass {
		return tool.Arguments{}, false, "", errors.New("agentexec: Tool approval prompt differs from its invocation")
	}
	resolution, err := interactioninput.Require(
		ctx,
		interrupt.Key(string(interrupt.Approval), request.ToolName, request.Arguments.Canonical()),
		pending,
	)
	if err != nil {
		return tool.Arguments{}, false, "", err
	}
	return o.resolveToolApproval(ctx, request, prompt, resolution)
}

func (o *observedInteractionTool) resumePreparedTool(
	ctx context.Context,
	callID string,
	name string,
	continued interactioninput.Continuation,
) (tool.Arguments, bool, string, error) {
	storedName, storedArguments := continued.Interrupt.Tool()
	if storedName != name {
		return tool.Arguments{}, false, "", errors.New("agentexec: continued Tool input belongs to another Tool")
	}
	arguments, err := tool.ParseArguments(storedArguments)
	if err != nil {
		return tool.Arguments{}, false, "", fmt.Errorf("agentexec: parse continued Tool arguments: %w", err)
	}
	if continued.Interrupt.Kind == interrupt.Question {
		return arguments, false, "", nil
	}
	if continued.Interrupt.Kind != interrupt.Approval || continued.Interrupt.Approval == nil {
		return tool.Arguments{}, false, "", errors.New("agentexec: continued Tool input has an unsupported kind")
	}
	prompt := *continued.Interrupt.Approval
	if prompt.CallID != callID {
		return tool.Arguments{}, false, "", errors.New("agentexec: continued Tool approval call identity changed")
	}
	request, err := o.authorizationRequest(callID, name, arguments, false)
	if err != nil {
		return arguments, false, "", err
	}
	return o.resolveToolApproval(ctx, request, prompt, continued.Resolution)
}

func (o *observedInteractionTool) resolveToolApproval(
	ctx context.Context,
	request ToolAuthorizationRequest,
	prompt runs.ApprovalPrompt,
	resolution interrupt.Resolution,
) (tool.Arguments, bool, string, error) {
	decision, err := o.authorizer.ResolveToolApproval(ctx, request, prompt, resolution)
	if err != nil {
		return tool.Arguments{}, false, "", interaction.HostFailure(fmt.Errorf("agentexec: resolve Tool %q approval: %w", request.ToolName, err))
	}
	arguments := request.Arguments
	if rewritten, ok := decision.EffectiveArguments(); ok {
		arguments = rewritten
	}
	reason, denied := decision.Denied()
	return arguments, denied, reason, nil
}

func (o *observedInteractionTool) activity(name string, arguments tool.Arguments) string {
	if o.presenter != nil {
		if activity := o.presenter.Activity(name, arguments); activity != "" && activity == strings.TrimSpace(activity) {
			return activity
		}
	}
	return "Calling " + name
}

func (o *observedInteractionTool) offload(
	ctx context.Context,
	toolName string,
	output corechat.ToolOutput,
	callErr error,
) (corechat.ToolOutput, *toolresult.Ref) {
	if callErr != nil {
		return output, nil
	}
	// The blob reader restores a single text body. Rich Content and its parallel
	// Details, citations, or metadata cannot be reconstructed by that contract.
	if len(output.Content) > 0 && len(output.Details) > 0 {
		return output, nil
	}
	for _, content := range output.Content {
		if len(content.Citations) > 0 || len(content.Metadata) > 0 {
			return output, nil
		}
	}
	text, textual := output.Text()
	if !textual {
		return output, nil
	}
	preview, reference := evictToolResult(
		ctx,
		o.offloader,
		o.offloadPolicy,
		o.start.SessionID,
		toolName,
		text,
	)
	if reference == nil {
		return output, nil
	}
	return corechat.NewTextToolOutput(preview), reference
}

func runtimeToolResult(output corechat.ToolOutput) (tool.Result, bool, error) {
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
		return tool.StringResult(text), true, nil
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return tool.Result{}, false, err
	}
	parsed, err := tool.ParseResult(encoded)
	return parsed, true, err
}

func hookToolOutput(output corechat.ToolOutput) (string, error) {
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
	manifest toolset.Manifest,
	session *interactionSession,
	config InteractionExecutorConfig,
	offloadPolicy toolResultOffloadPolicy,
	start runs.RootExecutionStart,
) (visible []toolcontract.Tool, deferred []toolcontract.Tool, err error) {
	if len(manifest.Visible)+len(manifest.Deferred) > 0 {
		if dependency.Missing(config.ToolInterpreter) {
			return nil, nil, errors.New("agentexec: Interaction Tools require a Tool interpreter")
		}
		if dependency.Missing(config.ToolAuthorizer) {
			return nil, nil, errors.New("agentexec: Interaction Tools require a Tool authorizer")
		}
	}
	wrap := func(values []toolcontract.Tool) ([]toolcontract.Tool, error) {
		wrapped := make([]toolcontract.Tool, len(values))
		for index, executable := range values {
			binding, bindErr := toolcontract.Bind(executable)
			if bindErr != nil {
				return nil, fmt.Errorf("agentexec: bind Interaction Tool[%d]: %w", index, bindErr)
			}
			name := binding.Contract().Definition().Name
			if class := config.ToolInterpreter.SafetyClass(name); !class.Valid() {
				return nil, fmt.Errorf("agentexec: Interaction Tool %q has invalid safety class %q", name, class)
			}
			observed := &observedInteractionTool{
				inner: executable, binding: binding, session: session, interpreter: config.ToolInterpreter,
				presenter: config.ToolPresenter, authorizer: config.ToolAuthorizer,
				hooks: config.ToolHooks, offloader: config.ToolResultStore,
				offloadPolicy: offloadPolicy,
				start:         start,
			}
			if config.ToolHooks == nil && !config.ToolInterpreter.UsesStandardPolicy(name) &&
				!slices.Contains(start.InterruptKinds, interrupt.Approval) {
				concurrent, _, err := toolcontract.Capability[interaction.ConcurrentTool](executable)
				if err != nil {
					return nil, fmt.Errorf("agentexec: resolve Tool concurrency: %w", err)
				}
				observed.concurrent = concurrent
			}
			wrapped[index] = observed
		}
		return wrapped, nil
	}
	visible, err = wrap(manifest.Visible)
	if err != nil {
		return nil, nil, err
	}
	deferred, err = wrap(manifest.Deferred)
	return visible, deferred, err
}
