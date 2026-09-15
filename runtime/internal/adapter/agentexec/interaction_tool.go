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
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
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
	return o.inner.Definition()
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
		var known *toolcontract.Failure
		if returnedErr != nil && !errors.Is(returnedErr, interaction.ErrToolInputRequired) && !errors.Is(returnedErr, toolcontract.ErrAuthorizationDenied) && !errors.As(returnedErr, &known) {
			o.session.effectFailures.record(invocation.EffectID(), returnedErr)
		}
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
		return corechat.ToolOutput{}, prepareErr
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
	if denied {
		metadata := toolResultMetadata{MemberID: member.MemberID, Start: start, Arguments: arguments.Canonical(), Failure: &tool.Failure{Kind: tool.FailureDenied, Detail: denialReason}}
		if err := o.session.rememberToolMetadata(metadata); err != nil {
			return corechat.ToolOutput{}, o.projectionFailure(err)
		}
		return corechat.ToolOutput{}, toolcontract.ErrAuthorizationDenied
	}
	ctx = toolset.WithToolAdvertiser(ctx, func(names ...string) error {
		return interaction.AdvertiseTools(ctx, names...)
	})
	var mutatedPaths []string
	ctx = toolset.WithMutationRecorder(ctx, func(paths []string) {
		mutatedPaths = append(mutatedPaths, paths...)
	})
	var output corechat.ToolOutput
	output, callErr := o.invoke(ctx, corechat.ToolCall{
		ID: call.ID, Name: call.Name, Arguments: rawArguments,
	})

	if errors.Is(callErr, interaction.ErrToolInputRequired) {
		// Tool input is an Interaction control boundary, not a failed external
		// call. The started fact remains open so the Run barrier can carry it as
		// a drained Tool; the restored invocation will commit the sole final fact
		// after consuming the semantic response Signal.
		return corechat.ToolOutput{}, callErr
	}
	var failure *toolcontract.Failure
	if callErr != nil && !errors.As(callErr, &failure) && !errors.Is(callErr, toolcontract.ErrAuthorizationDenied) {
		return corechat.ToolOutput{}, callErr
	}
	if failure != nil {
		output = failure.Output()
	}
	modelOutput, offload := o.offload(ctx, call.Name, output, callErr)
	metadata := toolResultMetadata{
		MemberID: member.MemberID, Start: start, Arguments: arguments.Canonical(),
		Offload: offload, MutatedPaths: normalizeMutationPaths(mutatedPaths),
	}
	if parsed, present := runtimeToolResult(modelOutput); present {
		if o.presenter != nil {
			parsed, metadata.OutputText = o.presenter.Present(call.Name, arguments, parsed)
		}
		metadata.Result = &parsed
	}
	if callErr != nil {
		metadata.Failure = &tool.Failure{Kind: tool.FailureExecution, Detail: executorDiagnostic(callErr)}
		if errors.Is(callErr, toolcontract.ErrAuthorizationDenied) {
			metadata.Failure = &tool.Failure{Kind: tool.FailureDenied}
		}
	}
	if err := o.session.rememberToolMetadata(metadata); err != nil {
		return corechat.ToolOutput{}, o.projectionFailure(err)
	}
	o.session.toolOutcomes.record(call.Name, arguments, modelOutput, callErr)
	o.projectToolOutcome(context.WithoutCancel(ctx), member, call.Name, callErr == nil)
	o.runAfterToolUseHook(ctx, callID, call.Name, arguments, modelOutput, callErr)
	return modelOutput, callErr
}

// invoke validates the effective arguments before entering the external Tool.
// This boundary can prove that an invalid edit never entered the executable.
func (o *observedInteractionTool) invoke(
	ctx context.Context,
	call corechat.ToolCall,
) (corechat.ToolOutput, error) {
	bound, err := o.binding.Contract().Prepare(call)
	if err != nil {
		cause := fmt.Errorf("agentexec: prepare Tool %q invocation: %w", call.Name, err)
		failure, failureErr := toolcontract.NewFailure(cause, corechat.NewTextToolOutput("invalid effective arguments: "+executorDiagnostic(cause)))
		if failureErr != nil {
			return corechat.ToolOutput{}, failureErr
		}
		return corechat.ToolOutput{}, failure
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
	rawArguments := string(bound.Arguments())
	if call.Name != o.Definition().Name || call.Arguments != rawArguments {
		return interaction.ToolInvocation{}, tool.Arguments{}, "", errors.New("agentexec: Tool invocation differs from its bound executable")
	}
	if _, err := conversation.NewToolCallIdentity(call.ID); err != nil {
		return interaction.ToolInvocation{}, tool.Arguments{}, "", fmt.Errorf("agentexec: Tool invocation: %w", err)
	}
	arguments, err := tool.ParseArguments(rawArguments)
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
	hookCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), authoritativeProjectionTimeout)
	if err := o.hooks.AfterToolUse(hookCtx, InteractionToolHookInput{
		SessionID: o.start.SessionID, CWD: o.start.CWD, CallID: callID,
		ToolName: name, Arguments: arguments, Result: hookToolOutput(output), CallError: callErr,
	}); err != nil {
		slog.ErrorContext(hookCtx, "agentexec: post-tool hook failed",
			"session.id", o.start.SessionID, "tool.name", name, "call.id", callID, "error", err,
		)
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
	if rewritten, ok := decision.EffectiveArguments(); ok {
		arguments = rewritten
	}
	if reason, denied := decision.Denied(); denied {
		return arguments, true, reason, nil
	}
	if prompt, ok := decision.Approval(); ok {
		return o.requestToolApproval(ctx, request, prompt)
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
			observed := &observedInteractionTool{
				inner: executable, binding: binding, session: session, interpreter: config.ToolInterpreter,
				presenter: config.ToolPresenter, authorizer: config.ToolAuthorizer,
				hooks: config.ToolHooks, offloader: config.ToolResultStore,
				offloadPolicy: offloadPolicy,
				start:         start,
			}
			if config.ToolHooks == nil && !config.ToolInterpreter.UsesStandardPolicy(executable.Definition().Name) &&
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

func validateToolManifest(manifest toolset.Manifest) error {
	seen := make(map[string]string, len(manifest.Visible)+len(manifest.Deferred))
	for _, group := range []struct {
		name   string
		values []toolcontract.Tool
	}{{name: "visible", values: manifest.Visible}, {name: "deferred", values: manifest.Deferred}} {
		name, values := group.name, group.values
		for index, executable := range values {
			if dependency.Missing(executable) {
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
