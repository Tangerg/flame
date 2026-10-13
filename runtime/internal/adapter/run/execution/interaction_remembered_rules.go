package execution

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	corechat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// DeriveRememberedRules validates each candidate's confirmed arguments against its
// frozen tool invocation on the staged tree and derives the canonical rule
// subject. A candidate whose arguments do not satisfy that invocation is omitted:
// its approval still proceeds one-shot and the Tool fails at execution, so no rule
// is remembered for an invocation that cannot run. Source authority is validated
// by the approval owner, and the returned requests are persisted by the acceptance
// transaction; nothing is written here.
func (i *InteractionExecutor) DeriveRememberedRules(
	ctx context.Context,
	ref runs.ExecutorRef,
	candidates []runs.RememberedRuleCandidate,
) ([]approval.RememberRequest, error) {
	session, err := i.session(ref)
	if err != nil {
		return nil, err
	}
	requests := make([]approval.RememberRequest, 0, len(candidates))
	for _, candidate := range candidates {
		executable, found, err := session.toolExecutable(candidate.Request.Tool)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("execution: remembered rule names unknown Tool %q", candidate.Request.Tool.ModelName())
		}
		subject, remembered, err := rememberedInvocationSubject(i.config.ToolInterpreter, executable, candidate)
		if err != nil {
			return nil, err
		}
		if !remembered {
			continue
		}
		request := candidate.Request
		request.Subject = approval.InvocationSubject(subject)
		requests = append(requests, request)
	}
	return requests, nil
}

// rememberedInvocationSubject reports the canonical subject for a candidate whose
// confirmed arguments satisfy the frozen invocation. It returns remembered=false
// when the arguments are invalid, which is how an edited approval that cannot run
// skips its rule without rejecting the Resume. Any other failure is a real error.
func rememberedInvocationSubject(
	interpreter InteractionToolInterpreter,
	executable toolcontract.Tool,
	candidate runs.RememberedRuleCandidate,
) (string, bool, error) {
	binding, err := toolcontract.Bind(executable)
	if err != nil {
		return "", false, fmt.Errorf("execution: bind remembered Tool: %w", err)
	}
	call := corechat.ToolCall{
		ID:        candidate.CallID,
		Name:      binding.Contract().Definition().Name,
		Arguments: candidate.EffectiveArguments.Canonical(),
	}
	if _, err := binding.Contract().Prepare(call); err != nil {
		return "", false, nil
	}
	subject, err := interpreter.ApprovalSubject(candidate.Request.Tool, candidate.EffectiveArguments)
	if err != nil {
		if errors.Is(err, tool.ErrInvalidArguments) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("execution: derive remembered Tool approval subject: %w", err)
	}
	return subject, true, nil
}

// toolExecutable finds the staged executable for one frozen Tool reference across
// this session's frozen manifests. A continuation derives a remembered rule from
// the exact Tool the approval was gated against, so the lookup is by reference, not
// by display name.
func (i *interactionSession) toolExecutable(ref tool.Ref) (toolcontract.Tool, bool, error) {
	i.state.mu.Lock()
	deployments := i.state.deployments
	i.state.mu.Unlock()
	if deployments == nil {
		return nil, false, nil
	}
	for _, manifest := range deployments.manifests {
		for _, executable := range append(slices.Clone(manifest.Visible), manifest.Deferred...) {
			identified, err := toolset.Identify(executable)
			if err != nil {
				return nil, false, err
			}
			if identified == ref {
				return executable, true, nil
			}
		}
	}
	return nil, false, nil
}
