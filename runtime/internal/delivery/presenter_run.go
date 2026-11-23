package delivery

import (
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func presentRunStatus(status rundomain.State) protocol.RunStatus {
	switch status {
	case rundomain.Running:
		return protocol.RunStatusRunning
	case rundomain.Waiting:
		return protocol.RunStatusWaiting
	case rundomain.Finished:
		return protocol.RunStatusFinished
	default:
		panic("delivery: unknown run status")
	}
}

func presentRunSummary(run rundomain.Run) protocol.RunSummary {
	summary := protocol.RunSummary{
		ID: run.ID().String(), SessionID: run.SessionID().String(), SpawnedByItemID: run.Lineage().SpawnedByItemID().String(),
		ParentRunID: run.Lineage().ParentRunID().String(), RootRunID: run.Lineage().RootRunID().String(),
		Provider: run.ModelSelection().Provider(), Model: run.ModelSelection().Model(),
		ReasoningEffort: run.ModelSelection().ReasoningEffort(),
		Status:          presentRunStatus(run.State()),
		CreatedAt:       run.CreatedAt(), FinishedAt: run.FinishedAt(),
	}
	if run.State().IsTerminal() {
		outcome := presentOutcome(run)
		summary.Outcome = &outcome
	}
	return summary
}

func presentRun(run rundomain.Run) protocol.RunRef {
	ref := protocol.RunRef{
		RunSummary:      presentRunSummary(run),
		ActiveSegmentID: run.ActiveSegmentID().String(),
		Metrics:         presentMetrics(run.Metrics()),
		ContextTokens:   run.ContextTokens(),
	}
	if run.Lineage().IsRoot() {
		capabilities := presentRunCapabilities(run.Capabilities())
		ref.Capabilities = &capabilities
	}
	return ref
}

func presentCancelResult(result runs.CancelResult) *protocol.CancelRunResponse {
	run := presentRun(result.Run)
	if result.RootRun == nil {
		if result.Run.Lineage().IsChild() {
			panic("delivery: child cancel result has no root run")
		}
		return &protocol.CancelRunResponse{Type: protocol.CancelRunRoot, Run: run}
	}
	if result.Run.Lineage().IsRoot() {
		panic("delivery: root cancel result unexpectedly carries a root run")
	}
	root := presentRun(*result.RootRun)
	return &protocol.CancelRunResponse{
		Type: protocol.CancelRunChild, Run: run, RootRun: &root,
	}
}

func presentRunCapabilities(capabilities rundomain.Capabilities) protocol.RunCapabilities {
	out := protocol.RunCapabilities{
		RequiredFeatures: make([]protocol.RunCapabilityFeature, 0, 1),
		InterruptTypes:   make([]protocol.InterruptType, 0, len(capabilities.InterruptTypes)),
	}
	if capabilities.ChildRuns {
		out.RequiredFeatures = append(out.RequiredFeatures, protocol.RunCapabilityFeatureChildRuns)
	}
	for _, kind := range capabilities.InterruptTypes {
		out.InterruptTypes = append(out.InterruptTypes, presentInterruptType(kind))
	}
	return out
}

func presentOutcome(run rundomain.Run) protocol.RunOutcome {
	ending, terminal := run.Ending()
	if !terminal {
		panic("delivery: terminal run has no outcome")
	}
	outcome := protocol.RunOutcome{UnresolvedEffects: presentUnresolvedEffects(run.UnresolvedEffects())}
	switch ending := ending.(type) {
	case rundomain.Completed:
		outcome.Type = protocol.OutcomeCompleted
	case rundomain.Canceled:
		outcome.Type, outcome.Detail = protocol.OutcomeCanceled, ending.Detail
	case rundomain.TimedOut:
		outcome.Type = protocol.OutcomeTimedOut
		outcome.Error = &protocol.ProblemData{Type: protocol.ProblemTimeout, Detail: ending.Detail}
	case rundomain.Failed:
		outcome.Type, outcome.Error = protocol.OutcomeFailed, presentRunFailure(&ending.Failure)
	case rundomain.Lost:
		outcome.Type, outcome.Loss = protocol.OutcomeLost, presentLoss(ending.Loss)
	default:
		panic("delivery: unknown run ending")
	}
	return outcome
}

var lossReasons = wireVocabulary[rundomain.LossReason, protocol.LossReason]{
	{rundomain.LossRestart, protocol.LossRestart},
	{rundomain.LossIsolatedWorkspace, protocol.LossIsolatedWorkspace},
	{rundomain.LossWorkspaceUnavailable, protocol.LossWorkspaceUnavailable},
	{rundomain.LossOtherBuild, protocol.LossOtherBuild},
	{rundomain.LossWaitingStateUnavailable, protocol.LossWaitingStateUnavailable},
	{rundomain.LossConfigurationChanged, protocol.LossConfigurationChanged},
	{rundomain.LossUnresolvedEffects, protocol.LossUnresolvedEffects},
	{rundomain.LossUnresolvedModelCall, protocol.LossUnresolvedModelCall},
}

var unresolvedCauses = wireVocabulary[rundomain.UnresolvedCause, protocol.UnresolvedEffectCause]{
	{rundomain.UnresolvedAtTermination, protocol.UnresolvedAtTermination},
	{rundomain.UnresolvedAtRuntimeFailure, protocol.UnresolvedAtRuntimeFailure},
}

var runFailureKinds = wireVocabulary[rundomain.FailureKind, protocol.ArtifactProblemType]{
	{rundomain.FailureInternal, protocol.ArtifactProblemInternalError},
	{rundomain.FailureAgentStuck, protocol.ArtifactProblemAgentStuck},
	{rundomain.FailureRateLimited, protocol.ArtifactProblemRateLimited},
	{rundomain.FailureInvalidCredentials, protocol.ArtifactProblemInvalidCredentials},
	{rundomain.FailureTimeout, protocol.ArtifactProblemTimeout},
	{rundomain.FailureProviderUnavailable, protocol.ArtifactProblemProviderUnavailable},
	{rundomain.FailureProviderRejected, protocol.ArtifactProblemProviderRejected},
}

var toolFailureKinds = wireVocabulary[tool.FailureKind, protocol.ArtifactProblemType]{
	{tool.FailureInternal, protocol.ArtifactProblemInternalError},
	{tool.FailureDenied, protocol.ArtifactProblemDeniedByUser},
	{tool.FailureExecution, protocol.ArtifactProblemToolFailed},
	{tool.FailureChildRunCanceled, protocol.ArtifactProblemChildRunCanceled},
	{tool.FailureCanceled, protocol.ArtifactProblemToolCanceled},
}

func presentLoss(loss rundomain.Loss) *protocol.RunLoss {
	return &protocol.RunLoss{Reason: lossReasons.mustWire(loss.Reason), Detail: loss.Detail}
}

func presentUnresolvedEffects(effects []rundomain.UnresolvedEffect) []protocol.UnresolvedEffect {
	projected := make([]protocol.UnresolvedEffect, 0, len(effects))
	for _, effect := range effects {
		projected = append(projected, protocol.UnresolvedEffect{ProcessID: effect.ProcessID(), EffectID: effect.EffectID(), Cause: unresolvedCauses.mustWire(effect.Cause()), Reason: effect.Reason(), Detail: effect.Detail(), Output: effect.Output()})
	}
	return projected
}

func presentMetrics(metrics rundomain.Metrics) protocol.RunMetrics {
	usage, reported := metrics.Usage()
	var usageRef *accounting.Usage
	if reported {
		usageRef = &usage
	}
	return protocol.RunMetrics{
		Usage:                presentUsage(usageRef),
		Steps:                metrics.Steps(),
		ActiveDurationMillis: metrics.ActiveDuration().Milliseconds(),
	}
}

func presentProgress(progress runs.Progress) protocol.RunProgress {
	return protocol.RunProgress{
		Step:  progress.Step,
		Usage: presentUsage(progress.Usage), ContextTokens: progress.ContextTokens,
		Activity: presentActivity(progress.Activity),
	}
}

func presentActivity(activity runs.Activity) *protocol.Activity {
	switch activity := activity.(type) {
	case runs.CallingModelActivity:
		return &protocol.Activity{Type: protocol.ActivityCallingModel}
	case runs.FailedModelCallActivity:
		return &protocol.Activity{Type: protocol.ActivityModelCallFailed}
	case runs.ApplyingUserInputActivity:
		return &protocol.Activity{Type: protocol.ActivityApplyingUserInput}
	case runs.ToolCallActivity:
		return &protocol.Activity{Type: protocol.ActivityToolCall, ItemID: activity.ItemID.String(), Tool: activity.Tool}
	default:
		return nil
	}
}

func presentUsage(usage *accounting.Usage) *protocol.Usage {
	if usage == nil {
		return nil
	}
	out := &protocol.Usage{ModelUsage: presentModelUsage(usage.Total())}
	if byModel := usage.ByModel(); len(byModel) > 0 {
		out.ByModel = make(map[string]protocol.ModelUsage, len(byModel))
		for model, modelUsage := range byModel {
			out.ByModel[model] = presentModelUsage(modelUsage)
		}
	}
	return out
}

func presentModelUsage(usage accounting.Totals) protocol.ModelUsage {
	return protocol.ModelUsage{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
		ReasoningTokens: usage.ReasoningTokens, CostUSD: usage.Cost.OptionalUSD(),
	}
}

func presentRunFailure(problem *rundomain.Failure) *protocol.ProblemData {
	if problem == nil {
		return nil
	}
	return &protocol.ProblemData{
		Type: string(runFailureKinds.mustWire(problem.Kind)), Detail: problem.Detail, DocURL: problem.DocURL,
		RetryAfterSeconds: problem.RetryAfterSeconds(),
	}
}

func presentToolFailure(failure *tool.Failure) *protocol.ProblemData {
	if failure == nil {
		return nil
	}
	return &protocol.ProblemData{
		Type: string(toolFailureKinds.mustWire(failure.Kind)), Detail: failure.Detail, DocURL: failure.DocURL,
	}
}

func presentInterrupts(interrupts []transcript.Interrupt) []protocol.Interrupt {
	out := make([]protocol.Interrupt, 0, len(interrupts))
	for _, request := range interrupts {
		entry := protocol.Interrupt{ItemID: request.ItemID.String(), RunID: request.RunID.String()}
		entry.Type = presentInterruptType(request.Type())
		switch prompt := request.Prompt.(type) {
		case transcript.Approval:
			entry.Payload = &protocol.InterruptPayload{
				Tool:         new(presentTool(prompt.Tool)),
				Risk:         presentApprovalRisk(prompt.Risk),
				Reason:       prompt.Reason,
				Rememberable: prompt.Rememberable,
			}
		case transcript.Question:
			entry.Payload = &protocol.InterruptPayload{Question: new(presentQuestion(prompt))}
		}
		out = append(out, entry)
	}
	return out
}
