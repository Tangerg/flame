package execution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func (i *interactionSession) investigateToolReceipts(ctx context.Context) (int, error) {
	if i.toolResults == nil {
		return 0, nil
	}
	root := i.state.processHandle()
	if root == nil {
		return 0, runs.ErrExecutorNotLive
	}
	tree, err := i.committedTree(ctx, root.ID().Root())
	if err != nil {
		return 0, err
	}
	inspection, err := i.logCache.inspect(tree)
	if err != nil {
		return 0, err
	}
	resolved := 0
	for _, snapshot := range inspection.Processes {
		if snapshot.Status.Terminal() {
			continue
		}
		if _, owned := i.state.deployments.interactions[snapshot.Program]; !owned {
			continue
		}
		for _, pending := range snapshot.PendingEffects {
			if pending.Outcome.Status() != scoperun.SettlementStatusUnknown {
				continue
			}
			confirmed, err := i.investigateToolReceipt(ctx, tree, pending.Request)
			if err != nil {
				return resolved, err
			}
			if confirmed {
				resolved++
			}
		}
	}
	return resolved, nil
}

func (i *interactionSession) retainedToolReceipt(tree scoperun.Log, request scoperun.EffectRequest) (toolResultMetadata, *observedInteractionTool, error) {
	_, toolEffect, err := interaction.ToolCallFromEffect(request.Effect())
	if err != nil || !toolEffect {
		return toolResultMetadata{}, nil, err
	}
	invocation, err := interaction.ToolInvocationFromRequest(request)
	if err != nil {
		return toolResultMetadata{}, nil, err
	}
	reference, err := toolProposalSource(tree, invocation.Reference(), invocation.ToolCall())
	if err != nil {
		return toolResultMetadata{}, nil, err
	}
	call := invocation.ToolCall()
	identity, err := logicalToolCallID(reference.process, reference.sequence, reference.index, call.ID, call.Name)
	if err != nil {
		return toolResultMetadata{}, nil, err
	}
	i.state.mu.Lock()
	metadata, retained := i.state.toolMetadata[identity.String()]
	deployments := i.state.deployments
	i.state.mu.Unlock()
	if !retained || metadata.Receipt == nil {
		return toolResultMetadata{}, nil, nil
	}
	proof := *metadata.Receipt
	owner := deployments.observedTools[request.ProgramRef()][proof.Tool]
	if owner == nil || owner.receiptEvidence == nil || owner.receiptEvidence.Tool != proof.Tool || owner.receiptEvidence.Release != proof.Release || owner.receiptEvidence.Authority != proof.Authority || request.ID().Process.String() != proof.ProcessID || request.ID().String() != proof.EffectID {
		return toolResultMetadata{}, nil, errors.New("execution: Tool receipt source differs from the captured Tool")
	}
	return metadata, owner, nil
}

func (i *interactionSession) investigateToolReceipt(ctx context.Context, tree scoperun.Log, request scoperun.EffectRequest) (bool, error) {
	metadata, owner, err := i.retainedToolReceipt(tree, request)
	if err != nil || owner == nil {
		return false, err
	}
	proof := *metadata.Receipt
	activation, err := toolset.InstallationActivation(owner.inner)
	if err != nil {
		return false, err
	}
	original, err := originalToolActivation(proof, []plugin.Activation{activation})
	if err != nil {
		return false, err
	}
	queryCtx, stop := context.WithTimeout(ctx, auxiliaryOperationTimeout)
	output, queryErr := i.toolResults.OriginalToolResult(queryCtx, proof.Tool, proof.Release, proof.Authority, original, metadata.Start.CallID, metadata.Start.Arguments)
	stop()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	// An unavailable, absent, conflicting, or malformed receipt proves nothing
	// about the original mutation. The Effect remains Unknown.
	failure, knownFailure := toolcontract.ClassifyFailure(queryErr)
	if queryErr != nil && !knownFailure || output == nil {
		if queryErr != nil {
			slog.WarnContext(ctx, "execution: original Tool result is inconclusive", "call.id", metadata.Start.CallID, "error", queryErr)
		}
		return false, nil
	}
	if knownFailure {
		value := failure.Output()
		output = &value
	}
	publicationCtx, stop := i.lifetime.publicationContext(ctx)
	defer stop()
	return i.publishToolReceipt(publicationCtx, request, owner, metadata, *output, queryErr)
}

func (i *interactionSession) publishToolReceipt(ctx context.Context, request scoperun.EffectRequest, owner *observedInteractionTool, metadata toolResultMetadata, output toolcontract.Output, outcomeErr error) (bool, error) {
	arguments, err := tool.ParseArguments(metadata.Start.Arguments)
	if err != nil {
		return false, err
	}
	projection, err := owner.projectToolResult(ctx, runs.ExecutorMember{MemberID: metadata.MemberID}, metadata.Start, arguments, output, outcomeErr, nil)
	if err != nil {
		return false, err
	}
	projected := projection.metadata
	projected.ProcessID = request.ID().Process.String()
	state := interaction.ResultSucceeded
	if projected.Failure != nil {
		state = interaction.ResultFailed
		if projected.Failure.Kind == tool.FailureDenied {
			state = interaction.ResultRejected
		}
	}
	if err := i.rememberToolMetadata(projected); err != nil {
		return false, err
	}
	settlement, err := interaction.SettleToolResult(i.state.deployments.interactions[request.ProgramRef()], request, toolcontract.Result{Output: projection.output}, state, nil)
	if err != nil {
		return false, err
	}
	process, found := i.engine.Process(request.ID().Process)
	if !found {
		return false, errors.New("execution: Unknown Effect owner is unavailable")
	}
	err = process.ResolveUnknownEffect(ctx, request.ID(), settlement)
	if errors.Is(err, scoperun.ErrProcessFinished) {
		result, awaitErr := process.Await(ctx)
		if awaitErr == nil && result.Termination().Status().Terminal() {
			return false, nil
		}
		err = errors.Join(err, awaitErr)
	}
	if err != nil {
		return false, fmt.Errorf("execution: publish original Tool result: %w", err)
	}
	return true, nil
}

func originalToolActivation(proof toolReceiptEvidence, bindings []plugin.Activation) (plugin.Activation, error) {
	server, _, found := proof.Tool.MCP()
	installation, installed := server.Origin().Installation()
	if !found || !installed {
		return plugin.Activation{}, plugin.ErrInvalid
	}
	return plugins.OriginalActivation(bindings, installation, proof.Release, server.Name())
}
