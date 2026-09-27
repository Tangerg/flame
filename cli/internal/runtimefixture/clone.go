package runtimefixture

import (
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
)

func projectRun(run *runState) conversation.Run {
	return conversation.Run{
		ID: run.id, SessionID: run.sessionID,
		Provider: run.provider, Model: run.model, ReasoningEffort: run.reasoningEffort,
		Lineage: run.lineage, Status: run.status, ActiveSegmentID: run.active,
		ContextTokens: run.contextTokens,
		Outcome:       run.outcome.Clone(), Usage: run.usage.Clone(),
	}
}
