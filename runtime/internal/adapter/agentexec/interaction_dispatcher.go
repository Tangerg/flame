package agentexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/flame/runtime/internal/adapter/agentexec/interactioninput"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/internal/domain/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/toolresult"
	"github.com/Tangerg/flame/runtime/internal/executoridentity"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

const authoritativeProjectionTimeout = 15 * time.Second

// interactionDispatcher gives each EffectRequest one independent attempt
// tracker. The inner Interaction Dispatcher still owns protocol decoding and
// definite settlements; this wrapper alone converts a post-external-call
// projection failure into Agent Framework's unknown settlement path.
type interactionDispatcher struct {
	inner   *interaction.Dispatcher
	session *interactionSession
}

func (i *interactionDispatcher) Dispatch(
	ctx context.Context,
	request agent.EffectRequest,
	emit agent.DeltaEmitter,
) (agent.Settlement, error) {
	ctx, finishDispatch := i.session.beginDispatch(ctx, request)
	defer finishDispatch()
	attempt := newDispatchAttempt(ctx, request.ID())
	defer attempt.close()
	settlement, err := i.inner.Dispatch(withDispatchAttempt(ctx, attempt), request, emit)
	if projectionErr := attempt.indeterminateFailure(); projectionErr != nil {
		i.session.lifetime.wakeUnknown()
		return agent.Settlement{}, fmt.Errorf(
			"agentexec: authoritative projection failed after external Effect %s: %w",
			request.ID(), projectionErr,
		)
	}
	if err != nil && attempt.crossedExternalBoundary() {
		// The inner Dispatcher already returns an indeterminate error to Engine.
		// Wake the direct path as well; the periodic public-state reconciliation
		// remains the loss-tolerant backstop.
		i.session.lifetime.wakeUnknown()
	}
	return settlement, err
}

func (i *interactionDispatcher) ReplayPolicy(effect agent.Effect) agent.ReplayPolicy {
	return i.inner.ReplayPolicy(effect)
}

type observedInteractionModel struct {
	inner   *chatclient.Client
	session *interactionSession
}

type observedInteractionCountingModel struct {
	*observedInteractionModel
}

func (o *observedInteractionCountingModel) CountInputTokens(
	ctx context.Context,
	request *corechat.Request,
) (int64, error) {
	return o.inner.CountInputTokens(ctx, request)
}

func (o *observedInteractionModel) Call(
	ctx context.Context,
	request *corechat.Request,
) (*corechat.Response, error) {
	invocation, attempt, callID, err := o.begin(ctx)
	if err != nil {
		return nil, err
	}
	if beginExternalCallErr := attempt.beginExternalCall(); beginExternalCallErr != nil {
		return nil, beginExternalCallErr
	}
	response, err := o.inner.Call(ctx, request)
	if err != nil {
		if projectionErr := o.fail(ctx, invocation, callID); projectionErr != nil {
			attempt.recordProjectionFailure(projectionErr)
			return nil, errors.Join(err, projectionErr)
		}
		return response, err
	}
	if response == nil {
		responseErr := errors.New("agentexec: model returned no response")
		if projectionErr := o.fail(ctx, invocation, callID); projectionErr != nil {
			attempt.recordProjectionFailure(projectionErr)
			return nil, errors.Join(responseErr, projectionErr)
		}
		return nil, responseErr
	}
	if err := response.Validate(); err != nil {
		if projectionErr := o.fail(ctx, invocation, callID); projectionErr != nil {
			attempt.recordProjectionFailure(projectionErr)
			return nil, errors.Join(err, projectionErr)
		}
		return response, err
	}
	if err := o.complete(ctx, invocation, callID, response); err != nil {
		attempt.recordProjectionFailure(err)
		return nil, err
	}
	return response, nil
}

func (o *observedInteractionModel) Stream(
	ctx context.Context,
	request *corechat.Request,
) iter.Seq2[*corechat.Response, error] {
	return func(yield func(*corechat.Response, error) bool) {
		invocation, attempt, callID, err := o.begin(ctx)
		if err != nil {
			yield(nil, err)
			return
		}
		if err := attempt.beginExternalCall(); err != nil {
			yield(nil, err)
			return
		}
		var accumulated corechat.ResponseAccumulator
		for chunk, streamErr := range o.inner.Stream(ctx, request) {
			if streamErr != nil {
				yield(nil, o.finishFailedStream(ctx, invocation, attempt, callID, streamErr))
				return
			}
			if err := accumulated.Add(chunk); err != nil {
				yield(nil, o.finishFailedStream(ctx, invocation, attempt, callID, err))
				return
			}
			if !yield(chunk, nil) {
				_ = o.finishFailedStream(ctx, invocation, attempt, callID, nil)
				return
			}
		}
		response := accumulated.Response()
		if response == nil {
			yield(nil, o.finishFailedStream(
				ctx,
				invocation,
				attempt,
				callID,
				errors.New("agentexec: model stream completed without a response"),
			))
			return
		}
		if err := response.Validate(); err != nil {
			yield(nil, o.finishFailedStream(ctx, invocation, attempt, callID, err))
			return
		}
		if err := o.complete(ctx, invocation, callID, response); err != nil {
			attempt.recordProjectionFailure(err)
			yield(nil, err)
		}
	}
}

func (o *observedInteractionModel) finishFailedStream(
	ctx context.Context,
	invocation interaction.ModelInvocation,
	attempt *dispatchAttempt,
	callID string,
	cause error,
) error {
	projectionErr := o.fail(ctx, invocation, callID)
	if projectionErr == nil {
		return cause
	}
	attempt.recordProjectionFailure(projectionErr)
	return errors.Join(cause, projectionErr)
}

func (o *observedInteractionModel) fail(
	ctx context.Context,
	invocation interaction.ModelInvocation,
	callID string,
) error {
	projectionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), authoritativeProjectionTimeout)
	defer cancel()
	return o.session.commitFact(
		projectionCtx,
		o.session.executorMember(invocation.Relation()),
		runs.ModelCallFailed{CallID: callID},
	)
}

func (o *observedInteractionModel) begin(
	ctx context.Context,
) (interaction.ModelInvocation, *dispatchAttempt, string, error) {
	invocation, ok := interaction.ModelInvocationFromContext(ctx)
	if !ok {
		return interaction.ModelInvocation{}, nil, "", errors.New("agentexec: model call has no Interaction attribution")
	}
	attempt, err := dispatchAttemptFrom(ctx, invocation.EffectID())
	if err != nil {
		return interaction.ModelInvocation{}, nil, "", err
	}
	callIdentity, err := modelInvocationID(invocation)
	if err != nil {
		return interaction.ModelInvocation{}, nil, "", err
	}
	callID := callIdentity.String()
	if _, err := o.session.reconcileCompletedDelegateChildren(ctx); err != nil {
		return interaction.ModelInvocation{}, nil, "", interaction.HostFailure(err)
	}
	member := o.session.executorMember(invocation.Relation())
	if err := o.session.commitAppliedSteers(ctx, member, invocation.AppliedSteerSignalIDs()); err != nil {
		return interaction.ModelInvocation{}, nil, "", interaction.HostFailure(err)
	}
	if err := o.session.commitFact(ctx, member, runs.ModelCallStarted{CallID: callID}); err != nil {
		return interaction.ModelInvocation{}, nil, "", interaction.HostFailure(
			fmt.Errorf("agentexec: commit model call start: %w", err),
		)
	}
	return invocation, attempt, callID, nil
}

func (o *observedInteractionModel) complete(
	ctx context.Context,
	invocation interaction.ModelInvocation,
	callID string,
	response *corechat.Response,
) error {
	modelOutput := response.Output
	if modelOutput == nil || modelOutput.Message == nil {
		return errors.New("agentexec: completed model call has no assistant message")
	}
	// Agent owns Delta validation, ordering, buffering, and listener observation. Wait on its
	// ordering barrier before committing the authoritative full response so an
	// accepted stream increment can never reopen an Item after completion.
	if err := o.session.flushDeltas(ctx); err != nil {
		return err
	}
	fact, err := o.session.accounting.accountModelCall(invocation, callID, response)
	if err != nil {
		return err
	}
	projectionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), authoritativeProjectionTimeout)
	defer cancel()
	if err := o.session.commitFact(
		projectionCtx, o.session.executorMember(invocation.Relation()), fact,
	); err != nil {
		return err
	}
	if !invocation.Relation().IsRoot() {
		o.session.committedReplies.record(invocation.Relation().ProcessID(), fact.Message)
	}
	return o.session.registerDelegateCalls(invocation, modelOutput.Message)
}

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
	rawArguments := string(bound.Arguments())
	invocation, ok := interaction.ToolInvocationFromContext(ctx)
	if !ok {
		return corechat.ToolOutput{}, errors.New("agentexec: Tool call has no Interaction attribution")
	}
	attempt, err := dispatchAttemptFrom(ctx, invocation.EffectID())
	if err != nil {
		return corechat.ToolOutput{}, err
	}
	call := invocation.ToolCall()
	if call.Name != o.Definition().Name || call.Arguments != rawArguments {
		return corechat.ToolOutput{}, errors.New("agentexec: Tool invocation differs from its bound executable")
	}
	if _, err := conversation.NewToolCallIdentity(call.ID); err != nil {
		return corechat.ToolOutput{}, fmt.Errorf("agentexec: Tool invocation: %w", err)
	}
	arguments, err := tool.ParseArguments(rawArguments)
	if err != nil {
		return corechat.ToolOutput{}, fmt.Errorf("agentexec: parse Tool %q arguments: %w", call.Name, err)
	}
	callIdentity, err := toolInvocationID(invocation)
	if err != nil {
		return corechat.ToolOutput{}, err
	}
	callID := callIdentity.String()
	ctx = interactioninput.WithCapabilities(ctx, o.start.InterruptKinds)
	arguments, denied, denialReason, err := o.prepare(ctx, callID, call.Name, arguments)
	if err != nil {
		return corechat.ToolOutput{}, err
	}
	rawArguments = arguments.Canonical()
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
		if denialReason == "" {
			denialReason = "tool call denied by policy"
		}
		denialOutput := corechat.NewTextToolOutput(denialReason)
		modelResult, _ := invocation.ModelResult(denialOutput, nil)
		end := o.finishedFact(
			callID, arguments, denialOutput, &modelResult, nil, nil, errors.New(denialReason),
		)
		end.Failure = &tool.Failure{
			Kind: tool.FailureDenied,
		}
		if err := o.session.commitFact(ctx, member, end); err != nil {
			failure := interaction.HostFailure(fmt.Errorf("agentexec: commit denied Tool result: %w", err))
			attempt.recordProjectionFailure(failure)
			return corechat.ToolOutput{}, failure
		}
		o.session.toolOutcomes.record(call.Name, arguments, denialOutput, errors.New(denialReason))
		return denialOutput, nil
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
	if o.interpreter != nil {
		projected, projectionErr := o.interpreter.ProjectOutcome(
			projectionCtx, o.start.SessionID, call.Name, callErr == nil,
		)
		if projectionErr != nil {
			trace.SpanFromContext(projectionCtx).RecordError(
				fmt.Errorf("agentexec: project Tool outcome: %w", projectionErr),
			)
		} else if projected != nil {
			// Tool outcome projection is a refetchable live hint (for example a Plan
			// snapshot), not a second settlement fact. The canonical Tool result above
			// is already committed; losing this hint cannot make the Effect unknown.
			o.session.lifetime.send(runs.ExecutorEvent{Member: member, Payload: projected})
		}
	}
	if o.hooks != nil {
		hookCtx, hookCancel := context.WithTimeout(context.WithoutCancel(ctx), authoritativeProjectionTimeout)
		if hookErr := o.hooks.AfterToolUse(hookCtx, InteractionToolHookInput{
			SessionID: o.start.SessionID, CWD: o.start.CWD, CallID: callID,
			ToolName: call.Name, Arguments: arguments, Result: hookToolOutput(modelOutput), CallError: callErr,
		}); hookErr != nil {
			trace.SpanFromContext(hookCtx).RecordError(
				fmt.Errorf("agentexec: run post-Tool hook: %w", hookErr),
			)
		}
		hookCancel()
	}
	return modelOutput, callErr
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
	if o.authorizer == nil || !o.interpreter.UsesStandardPolicy(name) {
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
	if o.authorizer == nil || !slices.Contains(o.start.InterruptKinds, interrupt.Approval) {
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
	if o.session != nil && o.session.mcpToolAutoApproved != nil {
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
	if o.authorizer == nil {
		return tool.Arguments{}, false, "", errors.New("agentexec: continued Tool approval has no authorizer")
	}
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

const (
	modelInvocationNamespace = "model"
	toolInvocationNamespace  = "tool"
)

func modelInvocationID(invocation interaction.ModelInvocation) (executoridentity.EffectID, error) {
	return modelInvocationIDFrom(invocation.EffectID(), invocation.ModelCallSequence())
}

func modelInvocationIDFrom(effectID agent.EffectID, modelCallSequence uint32) (executoridentity.EffectID, error) {
	digest := sha256.New()
	_, _ = digest.Write([]byte(effectID.String()))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(strconv.FormatUint(uint64(modelCallSequence), 10)))
	return parsedInvocationID(modelInvocationNamespace, digest.Sum(nil), modelCallSequence)
}

func toolInvocationID(invocation interaction.ToolInvocation) (executoridentity.EffectID, error) {
	return delegatedToolCallID(
		invocation.Relation(), invocation.ModelCallSequence(), invocation.ToolCallIndex(), invocation.ToolCall(),
	)
}

func delegatedToolCallID(
	relation agent.ProcessRelation,
	modelCallSequence uint32,
	toolCallIndex uint32,
	call corechat.ToolCall,
) (executoridentity.EffectID, error) {
	digest := sha256.New()
	_, _ = digest.Write([]byte(relation.ProcessID().String()))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(strconv.FormatUint(uint64(modelCallSequence), 10)))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(call.ID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(call.Name))
	return parsedInvocationID(toolInvocationNamespace, digest.Sum(nil), toolCallIndex)
}

func parsedInvocationID(namespace string, digest []byte, ordinal uint32) (executoridentity.EffectID, error) {
	return executoridentity.ParseEffect(
		namespace + ":" + hex.EncodeToString(digest) + ":" + strconv.FormatUint(uint64(ordinal), 10),
	)
}

func basicExecutorMember(relation agent.ProcessRelation) runs.ExecutorMember {
	member := runs.ExecutorMember{MemberID: relation.ProcessID().String()}
	if parentID, child := relation.ParentID(); child {
		member.ParentID = parentID.String()
	}
	return member
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

func newObservedInteractionClient(
	inner *chatclient.Client,
	session *interactionSession,
) (*chatclient.Client, error) {
	observed := &observedInteractionModel{inner: inner, session: session}
	var model corechat.Model = observed
	if inner.SupportsInputTokenCounting() {
		model = &observedInteractionCountingModel{observedInteractionModel: observed}
	}
	client, err := chatclient.New(model, chatclient.Config{Streamer: observed})
	if err != nil {
		return nil, err
	}
	return &client, nil
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

func modelUsage(
	response *corechat.Response,
	selection modelref.Selection,
	pricing accounting.Pricing,
) accounting.ModelUsage {
	var metadata corechat.ResponseMetadata
	if response.Metadata != nil {
		metadata = *response.Metadata
	}
	servedModel := metadata.Model
	if servedModel == "" {
		servedModel = selection.Model()
	}
	cost := 0.0
	if pricing != nil {
		cost = pricing(selection.Provider(), servedModel, &metadata.Usage)
	}
	return accounting.ModelUsage{
		Model: servedModel, TokenUsage: accountingTokenUsage(metadata.Usage), CostUSD: cost, Calls: 1,
	}
}

func accountingTokenUsage(usage corechat.Usage) accounting.TokenUsage {
	result := accounting.TokenUsage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
	}
	if usage.ReasoningTokens != nil {
		result.ReasoningTokens = *usage.ReasoningTokens
	}
	if usage.CacheReadInputTokens != nil {
		result.CacheReadTokens = *usage.CacheReadInputTokens
	}
	if usage.CacheWriteInputTokens != nil {
		result.CacheWriteTokens = *usage.CacheWriteInputTokens
	}
	return result
}
