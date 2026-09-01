package runtimefixture

import "github.com/Tangerg/flame/cli/internal/domain/agent"

func projectRun(run *runState) agent.Run {
	return agent.Run{
		ID: run.id, SessionID: run.sessionID,
		Provider: run.provider, Model: run.model, ReasoningEffort: run.reasoningEffort,
		Lineage: run.lineage, Status: run.status, ActiveSegmentID: run.active,
		Limits: run.limits, ContextTokens: run.contextTokens,
		Outcome: run.outcome.Clone(), Usage: run.usage.Clone(),
	}
}
