package execution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
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
	tree, err := i.engine.CaptureTree(ctx, root.Relation().RootID())
	if err != nil {
		return 0, err
	}
	resolved := 0
	for _, snapshot := range tree.ProcessSnapshots() {
		if snapshot.Status().Terminal() {
			continue
		}
		for _, id := range snapshot.UnknownEffectIDs() {
			request, found := tree.EffectRequest(snapshot.Relation().ProcessID(), id)
			if !found {
				return resolved, errors.New("execution: Unknown Effect has no retained request")
			}
			invocation, err := interaction.ToolInvocationFromRequest(request)
			if err != nil {
				continue
			}
			reference, found := invocation.Reference()
			if !found {
				return resolved, errors.New("execution: retained Tool request has no attribution")
			}
			call := invocation.ToolCall()
			identity, err := logicalToolCallID(reference.ProcessID(), reference.ModelCallSequence(), reference.ToolCallIndex(), call.ID, call.Name)
			if err != nil {
				return resolved, err
			}
			i.state.mu.Lock()
			metadata, retained := i.state.toolMetadata[identity.String()]
			deployments := i.state.deployments
			i.state.mu.Unlock()
			if !retained || metadata.Receipt == nil {
				continue
			}
			proof := *metadata.Receipt
			owner := deployments.observedTools[request.DeploymentRef()][proof.Tool]
			if owner == nil || owner.receiptEvidence == nil || owner.receiptEvidence.Tool != proof.Tool || owner.receiptEvidence.Release != proof.Release || owner.receiptEvidence.Authority != proof.Authority || request.Relation().ProcessID().String() != proof.ProcessID || request.ID().String() != proof.EffectID {
				return resolved, errors.New("execution: Tool receipt source differs from the captured Tool")
			}
			queryCtx, stop := context.WithTimeout(ctx, auxiliaryOperationTimeout)
			output, queryErr := i.toolResults.OriginalToolResult(queryCtx, proof.Tool, proof.Release, proof.Authority, metadata.Start.CallID, metadata.Start.Arguments)
			stop()
			if err := ctx.Err(); err != nil {
				return resolved, err
			}
			// An unavailable, absent, conflicting, or malformed receipt proves
			// nothing about the original mutation. The Effect remains Unknown.
			failure, knownFailure := toolcontract.ClassifyFailure(queryErr)
			if queryErr != nil && !knownFailure || output == nil {
				if queryErr != nil {
					slog.WarnContext(ctx, "execution: original Tool result is inconclusive", "call.id", metadata.Start.CallID, "error", queryErr)
				}
				continue
			}
			if knownFailure {
				value := failure.Output()
				output = &value
			}
			arguments, err := tool.ParseArguments(metadata.Start.Arguments)
			if err != nil {
				return resolved, err
			}
			publicationCtx, stop := i.lifetime.publicationContext(ctx)
			modelOutput, projected, err := owner.resultMetadata(publicationCtx, runs.ExecutorMember{MemberID: metadata.MemberID}, metadata.Start, arguments, *output, queryErr, nil)
			state := interaction.ResultSucceeded
			if knownFailure {
				state = interaction.ResultFailed
				projected.Failure = &tool.Failure{Kind: tool.FailureExecution, Detail: executorDiagnostic(queryErr)}
			}
			if err == nil {
				err = i.rememberToolMetadata(projected)
			}
			var settlement agent.Settlement
			if err == nil {
				settlement, err = deployments.toolSets[request.DeploymentRef()].SettleToolResult(request, modelOutput, state, nil)
			}
			process, found := i.engine.Process(request.Relation().ProcessID())
			if err == nil && !found {
				err = errors.New("execution: Unknown Effect owner is unavailable")
			}
			if err == nil {
				err = process.ResolveUnknownEffect(publicationCtx, id, settlement)
			}
			if errors.Is(err, agent.ErrProcessFinished) {
				result, awaitErr := process.Await(publicationCtx)
				if awaitErr == nil && result.Termination().Status().Terminal() {
					stop()
					continue
				}
				err = errors.Join(err, awaitErr)
			}
			stop()
			if err != nil {
				return resolved, fmt.Errorf("execution: publish original Tool result: %w", err)
			}
			resolved++
		}
	}
	return resolved, nil
}
