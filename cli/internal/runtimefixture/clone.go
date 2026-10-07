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

// boundaryRun is the record a segment ends with. The fixture still holds the
// active segment to publish the frame, but the Run it describes has left it.
func boundaryRun(run *runState) conversation.Run {
	record := projectRun(run)
	record.ActiveSegmentID = ""
	return record
}
