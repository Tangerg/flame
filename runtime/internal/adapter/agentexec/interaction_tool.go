package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/flame/runtime/internal/adapter/agentexec/interactioninput"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	"github.com/Tangerg/scope/agent/interaction"
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
}

func (o *observedInteractionTool) Definition() corechat.ToolDefinition {
	return o.inner.Definition()
}

func (o *observedInteractionTool) Unwrap() toolcontract.Tool { return o.inner }

func (o *observedInteractionTool) Call(ctx context.Context, bound toolcontract.Invocation) (corechat.ToolOutput, error) {
	invocation, attempt, arguments, callID, err := o.attributedInvocation(ctx, bound)
	if err != nil {
		return corechat.ToolOutput{}, err
	}
	call := invocation.ToolCall()
	ctx = interactioninput.WithCapabilities(ctx, o.start.InterruptKinds)
	arguments, denied, denialReason, err := o.prepare(ctx, callID, call.Name, arguments)
	if err != nil {
		return corechat.ToolOutput{}, err
	}
	rawArguments := arguments.Canonical()
	innerInvocation, err := o.binding.Prepare(corechat.ToolCall{
		ID: call.ID, Name: call.Name, Arguments: rawArguments,
	})
	if err != nil {
		return corechat.ToolOutput{}, fmt.Errorf("agentexec: prepare Tool %q invocation: %w", call.Name, err)
	}
	member := o.session.executorMember(invocation.Relation())
	start := runs.ToolCallStarted{
		CallID: callID, ModelCallSequence: invocation.ModelCallSequence(),
		ToolCallIndex: invocation.ToolCallIndex(), SourceCallID: call.ID, ToolName: call.Name,
		Arguments: rawArguments, Activity: o.activity(call.Name, arguments),
		SafetyClass: o.interpreter.SafetyClass(call.Name),
	}
	if err := o.session.commitFact(ctx, member, start); err != nil {
		failure := interaction.HostFailure(fmt.Errorf("agentexec: commit Tool call start: %w", err))
		attempt.recordProjectionFailure(failure)
		return corechat.ToolOutput{}, failure
	}
	o.session.accounting.recordToolCall()
	if denied {
		return o.settleDeniedToolCall(ctx, attempt, invocation, member, callID, arguments, denialReason)
	}
	ctx = toolset.WithToolAdvertiser(ctx, func(names ...string) error {
		return interaction.AdvertiseTools(ctx, names...)
	})
	if err := attempt.beginExternalCall(); err != nil {
		return corechat.ToolOutput{}, err
	}
	var mutatedPaths []string
	ctx = toolset.WithMutationRecorder(ctx, func(paths []string) {
		mutatedPaths = append(mutatedPaths, paths...)
	})
	output, callErr := o.binding.Call(ctx, innerInvocation)
	if attemptErr := attempt.indeterminateFailure(); attemptErr != nil {
		return corechat.ToolOutput{}, attemptErr
	}
	if errors.Is(context.Cause(ctx), errInteractionRunCanceled) &&
		(errors.Is(callErr, context.Canceled) || errors.Is(callErr, context.DeadlineExceeded)) {
		// The product cancellation plane, unlike an arbitrary caller deadline,
		// has already accepted terminal intent and owns the durable Tool fact
		// committed below. Return a definite Tool failure to Interaction so Agent
		// can settle the in-flight Effect and apply that intent at its safe
		// boundary; retaining context.Canceled here would correctly-but-uselessly
		// classify the whole Effect as unknown.
		callErr = errInteractionRunCanceled
	}
	if _, ok := errors.AsType[*interaction.ToolInputRequiredError](callErr); ok {
		// Tool input is an Interaction control boundary, not a failed external
		// call. The started fact remains open so the Run barrier can carry it as
		// a drained Tool; the restored invocation will commit the sole final fact
		// after consuming the semantic response Signal.
		return corechat.ToolOutput{}, callErr
	}
	modelOutput, offload := o.offload(ctx, call.Name, output, callErr)
	modelResult, modelResultPresent := invocation.ModelResult(modelOutput, callErr)
	var exactModelResult *corechat.ToolResult
	if modelResultPresent {
		exactModelResult = &modelResult
	}
	end := o.finishedFact(
		callID,
		arguments,
		modelOutput,
		exactModelResult,
		offload,
		normalizeMutationPaths(mutatedPaths),
		callErr,
	)
	// A later concurrent Tool may finish before an earlier model-declared call.
	// Its commit receipt intentionally waits for the canonical durable prefix;
	// the Effect context and executor release own that wait, not an arbitrary local
	// timeout that could misclassify a healthy long-running sibling as unknown.
	projectionCtx, cancelProjection := attempt.projectionContext(context.WithoutCancel(ctx))
	defer cancelProjection()
	commitErr := o.session.commitFact(projectionCtx, member, end)
	if commitErr != nil {
		attempt.recordProjectionFailure(commitErr)
		return corechat.ToolOutput{}, fmt.Errorf("agentexec: commit Tool result: %w", commitErr)
	}
	o.session.toolOutcomes.record(call.Name, arguments, modelOutput, callErr)
	o.projectToolOutcome(projectionCtx, member, call.Name, callErr == nil)
	o.runAfterToolUseHook(ctx, callID, call.Name, arguments, modelOutput, callErr)
	return modelOutput, callErr
}

func (o *observedInteractionTool) attributedInvocation(
	ctx context.Context,
	bound toolcontract.Invocation,
) (interaction.ToolInvocation, *dispatchAttempt, tool.Arguments, string, error) {
	invocation, ok := interaction.ToolInvocationFromContext(ctx)
	if !ok {
		return interaction.ToolInvocation{}, nil, tool.Arguments{}, "", errors.New("agentexec: Tool call has no Interaction attribution")
	}
	attempt, err := dispatchAttemptFrom(ctx, invocation.EffectID())
	if err != nil {
		return interaction.ToolInvocation{}, nil, tool.Arguments{}, "", err
	}
	call := invocation.ToolCall()
	rawArguments := string(bound.Arguments())
	if call.Name != o.Definition().Name || call.Arguments != rawArguments {
		return interaction.ToolInvocation{}, nil, tool.Arguments{}, "", errors.New("agentexec: Tool invocation differs from its bound executable")
	}
	if _, err := conversation.NewToolCallIdentity(call.ID); err != nil {
		return interaction.ToolInvocation{}, nil, tool.Arguments{}, "", fmt.Errorf("agentexec: Tool invocation: %w", err)
	}
	arguments, err := tool.ParseArguments(rawArguments)
	if err != nil {
		return interaction.ToolInvocation{}, nil, tool.Arguments{}, "", fmt.Errorf("agentexec: parse Tool %q arguments: %w", call.Name, err)
	}
	callIdentity, err := toolInvocationID(invocation)
	if err != nil {
		return interaction.ToolInvocation{}, nil, tool.Arguments{}, "", err
	}
	return invocation, attempt, arguments, callIdentity.String(), nil
}

func (o *observedInteractionTool) settleDeniedToolCall(
	ctx context.Context,
	attempt *dispatchAttempt,
	invocation interaction.ToolInvocation,
	member runs.ExecutorMember,
	callID string,
	arguments tool.Arguments,
	reason string,
) (corechat.ToolOutput, error) {
	if reason == "" {
		reason = "tool call denied by policy"
	}
	denialOutput := corechat.NewTextToolOutput(reason)
	modelResult, _ := invocation.ModelResult(denialOutput, nil)
	end := o.finishedFact(callID, arguments, denialOutput, &modelResult, nil, nil, errors.New(reason))
	end.Failure = &tool.Failure{Kind: tool.FailureDenied}
	if err := o.session.commitFact(ctx, member, end); err != nil {
		failure := interaction.HostFailure(fmt.Errorf("agentexec: commit denied Tool result: %w", err))
		attempt.recordProjectionFailure(failure)
		return corechat.ToolOutput{}, failure
	}
	o.session.toolOutcomes.record(invocation.ToolCall().Name, arguments, denialOutput, errors.New(reason))
	return denialOutput, nil
}

func (o *observedInteractionTool) projectToolOutcome(
	ctx context.Context,
	member runs.ExecutorMember,
	name string,
	succeeded bool,
) {
	projected, err := o.interpreter.ProjectOutcome(ctx, o.start.SessionID, name, succeeded)
	if err != nil {
		trace.SpanFromContext(ctx).RecordError(fmt.Errorf("agentexec: project Tool outcome: %w", err))
		return
	}
	if projected != nil {
		// Tool outcome projection is a refetchable live hint (for example a Plan
		// snapshot), not a second settlement fact. The canonical Tool result is
		// already committed; losing this hint cannot make the Effect unknown.
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
	hookCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), authoritativeProjectionTimeout)
	if err := o.hooks.AfterToolUse(hookCtx, InteractionToolHookInput{
		SessionID: o.start.SessionID, CWD: o.start.CWD, CallID: callID,
		ToolName: name, Arguments: arguments, Result: hookToolOutput(output), CallError: callErr,
	}); err != nil {
		trace.SpanFromContext(hookCtx).RecordError(fmt.Errorf("agentexec: run post-Tool hook: %w", err))
	}
	cancel()
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
			SessionID: o.start.SessionID, CWD: o.start.CWD,
			CallID: callID, ToolName: name, Arguments: arguments,
		})
		if beforeToolUseErr != nil {
			return tool.Arguments{}, false, "", fmt.Errorf("agentexec: run pre-Tool hook: %w", beforeToolUseErr)
		}
		if validateHookDecisionErr := validateHookDecision(decision); validateHookDecisionErr != nil {
			return tool.Arguments{}, false, "", validateHookDecisionErr
		}
		if decision.EffectiveArguments != nil {
			arguments = *decision.EffectiveArguments
		}
		if decision.Denied {
			return arguments, true, decision.Reason, nil
		}
		forceApproval = decision.RequireApproval
	}
	if !o.interpreter.UsesStandardPolicy(name) {
		if forceApproval {
			return arguments, true, "a lifecycle hook requires approval, but approval is unavailable", nil
		}
		return o.applyDoomLoopBrake(ctx, callID, name, arguments, false, "")
	}
	request, err := o.authorizationRequest(callID, name, arguments, forceApproval)
	if err != nil {
		return tool.Arguments{}, false, "", err
	}
	decision, err := o.authorizer.AuthorizeTool(ctx, request)
	if err != nil {
		return tool.Arguments{}, false, "", fmt.Errorf("agentexec: authorize Tool %q: %w", name, err)
	}
	if err := validateToolAuthorizationDecision(decision); err != nil {
		return tool.Arguments{}, false, "", err
	}
	if decision.EffectiveArguments != nil {
		arguments = *decision.EffectiveArguments
	}
	if decision.Denied {
		return arguments, true, decision.Reason, nil
	}
	if decision.Approval != nil {
		return o.requestToolApproval(ctx, request, *decision.Approval)
	}
	return o.applyDoomLoopBrake(ctx, callID, name, arguments, false, "")
}

func (o *observedInteractionTool) applyDoomLoopBrake(
	ctx context.Context,
	callID string,
	name string,
	arguments tool.Arguments,
	denied bool,
	reason string,
) (tool.Arguments, bool, string, error) {
	if denied || o.session.toolOutcomes.repeated(name, arguments) < interactionDoomLoopThreshold {
		return arguments, denied, reason, nil
	}
	o.session.toolOutcomes.reset()
	reason = fmt.Sprintf(
		"%q has been called with the same arguments and unchanged result %d times; approve to continue or deny so the agent changes approach",
		name, interactionDoomLoopThreshold,
	)
	if !slices.Contains(o.start.InterruptKinds, interrupt.Approval) {
		return arguments, true, reason, nil
	}
	request, err := o.authorizationRequest(callID, name, arguments, true)
	if err != nil {
		return tool.Arguments{}, false, "", err
	}
	return o.requestToolApproval(ctx, request, runs.ApprovalPrompt{
		CallID: callID, ToolName: name, Arguments: arguments.Canonical(),
		SafetyClass: request.SafetyClass, Risk: tool.RiskHigh, Reason: reason,
	})
}

func (o *observedInteractionTool) authorizationRequest(
	callID string,
	name string,
	arguments tool.Arguments,
	requireApproval bool,
) (ToolAuthorizationRequest, error) {
	subject, err := o.interpreter.ApprovalSubject(name, arguments)
	if err != nil {
		return ToolAuthorizationRequest{}, fmt.Errorf("agentexec: derive Tool %q approval subject: %w", name, err)
	}
	autoApproved := false
	if o.session.mcpToolAutoApproved != nil {
		if identity, ok := o.inner.(interactionMCPToolIdentity); ok {
			server, remote := identity.MCPToolIdentity()
			autoApproved = server != "" && remote != "" && o.session.mcpToolAutoApproved(server, remote)
		}
	}
	return ToolAuthorizationRequest{
		SessionID: o.start.SessionID, CWD: o.start.CWD,
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
		interrupt.Key(interrupt.Approval.String(), request.ToolName, request.Arguments.Canonical()),
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
		return tool.Arguments{}, false, "", err
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
		return tool.Arguments{}, false, "", fmt.Errorf("agentexec: resolve Tool %q approval: %w", request.ToolName, err)
	}
	if err := validateToolAuthorizationDecision(decision); err != nil {
		return tool.Arguments{}, false, "", err
	}
	arguments := request.Arguments
	if decision.EffectiveArguments != nil {
		arguments = *decision.EffectiveArguments
	}
	return arguments, decision.Denied, decision.Reason, nil
}

func (o *observedInteractionTool) activity(name string, arguments tool.Arguments) string {
	if o.presenter != nil {
		if activity := o.presenter.Activity(name, arguments); activity != "" && activity == strings.TrimSpace(activity) {
			return activity
		}
	}
	return "Calling " + name
}

func (o *observedInteractionTool) finishedFact(
	callID string,
	arguments tool.Arguments,
	output corechat.ToolOutput,
	modelResult *corechat.ToolResult,
	offload *toolresult.Ref,
	mutatedPaths []string,
	callErr error,
) runs.ToolCallFinished {
	var result *tool.Result
	var exactModelResult *corechat.ToolResult
	if modelResult != nil {
		value := *modelResult
		exactModelResult = &value
	}
	outputText := ""
	if parsed, present := runtimeToolResult(output); present {
		if o.presenter != nil {
			parsed, outputText = o.presenter.Present(o.Definition().Name, arguments, parsed)
		}
		result = &parsed
	}
	finished := runs.ToolCallFinished{
		CallID: callID, Arguments: arguments.Canonical(), ModelResult: exactModelResult, Result: result,
		Offload: offload, OutputText: outputText, MutatedPaths: slices.Clone(mutatedPaths),
	}
	if callErr != nil {
		finished.Failure = &tool.Failure{
			Kind:   tool.FailureExecution,
			Detail: callErr.Error(),
		}
		if errors.Is(callErr, errInteractionRunCanceled) {
			// The symbolic cancellation kind is the client-visible explanation.
			// Keeping the adapter sentinel out of Detail lets each consumer own
			// localized presentation instead of exposing implementation vocabulary.
			finished.Failure = &tool.Failure{Kind: tool.FailureCanceled}
		}
	}
	return finished
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

func runtimeToolResult(output corechat.ToolOutput) (tool.Result, bool) {
	if len(output.Content) == 0 && len(output.Details) == 0 {
		return tool.Result{}, false
	}
	if len(output.Details) > 0 {
		if parsed, err := tool.ParseResult(output.Details); err == nil {
			return parsed, true
		}
	}
	if text, textual := output.Text(); textual {
		if parsed, err := tool.ParseResult([]byte(text)); err == nil {
			return parsed, true
		}
		return tool.StringResult(text), true
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return tool.StringResult("[invalid non-text tool output]"), true
	}
	parsed, err := tool.ParseResult(encoded)
	if err != nil {
		return tool.StringResult("[non-text tool output]"), true
	}
	return parsed, true
}

func hookToolOutput(output corechat.ToolOutput) string {
	if text, textual := output.Text(); textual {
		return text
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return "[invalid non-text tool output]"
	}
	return string(encoded)
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
	wrap := func(values []toolcontract.Tool) ([]toolcontract.Tool, error) {
		wrapped := make([]toolcontract.Tool, len(values))
		for index, executable := range values {
			binding, bindErr := toolcontract.Bind(executable)
			if bindErr != nil {
				return nil, fmt.Errorf("agentexec: bind Interaction Tool %q: %w", executable.Definition().Name, bindErr)
			}
			wrapped[index] = &observedInteractionTool{
				inner: executable, binding: binding, session: session, interpreter: config.ToolInterpreter,
				presenter: config.ToolPresenter, authorizer: config.ToolAuthorizer,
				hooks: config.ToolHooks, offloader: config.ToolResultStore,
				offloadPolicy: offloadPolicy,
				start:         start,
			}
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

func validateToolManifest(manifest toolset.Manifest) error {
	seen := make(map[string]string, len(manifest.Visible)+len(manifest.Deferred))
	for _, group := range []struct {
		name   string
		values []toolcontract.Tool
	}{{name: "visible", values: manifest.Visible}, {name: "deferred", values: manifest.Deferred}} {
		name, values := group.name, group.values
		for index, executable := range values {
			if isNilInteractionCapability(executable) {
				return fmt.Errorf("agentexec: %s Interaction Tool[%d] is nil", name, index)
			}
			toolName := executable.Definition().Name
			if strings.TrimSpace(toolName) == "" || toolName != strings.TrimSpace(toolName) {
				return fmt.Errorf("agentexec: %s Interaction Tool[%d] has an invalid name", name, index)
			}
			if prior, duplicate := seen[toolName]; duplicate {
				return fmt.Errorf(
					"agentexec: Interaction Tool %q appears more than once (first in %s, again in %s)",
					toolName,
					prior,
					name,
				)
			}
			seen[toolName] = name
		}
	}
	return nil
}

func isNilInteractionCapability(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
