package runtimefixture

import (
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/runtime/protocol"
)

func projectRun(run *runState) protocol.RunRef {
	return protocol.RunRef{
		RunSummary: protocol.RunSummary{
			ID: run.id, SessionID: run.sessionID,
			Provider: run.provider, Model: run.model, ReasoningEffort: run.reasoningEffort,
			Status: run.status, Outcome: run.outcome.RunOutcome(),
		},
		ActiveSegmentID: run.active, Limits: run.limits.Protocol(),
		ContextTokens: run.contextTokens, Metrics: agent.CloneRunMetrics(run.metrics),
	}
}
