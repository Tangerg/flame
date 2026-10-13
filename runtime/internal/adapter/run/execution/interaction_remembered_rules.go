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
// subject. An invalid invocation rejects the requested remembered rule and the
// complete Resume. Source authority is validated by the approval owner; the
// acceptance transaction persists the returned write-set.
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
		subject, err := rememberedInvocationSubject(i.config.ToolInterpreter, executable, candidate)
		if err != nil {
			return nil, err
		}
		request := candidate.Request
		request.Subject = approval.InvocationSubject(subject)
		requests = append(requests, request)
	}
	return requests, nil
}

// rememberedInvocationSubject admits the exact confirmed invocation before any
// rule or answer can commit. It never downgrades a remember request to one-shot.
func rememberedInvocationSubject(
	interpreter InteractionToolInterpreter,
	executable toolcontract.Tool,
	candidate runs.RememberedRuleCandidate,
) (string, error) {
	binding, err := toolcontract.Bind(executable)
	if err != nil {
		return "", fmt.Errorf("execution: bind remembered Tool: %w", err)
	}
	call := corechat.ToolCall{
		ID:        candidate.CallID,
		Name:      binding.Contract().Definition().Name,
		Arguments: candidate.EffectiveArguments.Canonical(),
	}
	if _, err := binding.Contract().Prepare(call); err != nil {
		return "", fmt.Errorf("%w: remembered invocation does not satisfy its Tool contract: %w", runs.ErrInvalidInterruptResponse, err)
	}
	subject, err := interpreter.ApprovalSubject(candidate.Request.Tool, candidate.EffectiveArguments)
	if err != nil {
		if errors.Is(err, tool.ErrInvalidArguments) {
			return "", fmt.Errorf("%w: remembered invocation arguments: %w", runs.ErrInvalidInterruptResponse, err)
		}
		return "", fmt.Errorf("execution: derive remembered Tool approval subject: %w", err)
	}
	return subject, nil
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
