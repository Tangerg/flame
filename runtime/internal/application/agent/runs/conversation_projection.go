package runs

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	corechat "github.com/Tangerg/scope/core/chat"
)

type ToolResultPublications interface {
	UnpublishedToolResults(ctx context.Context, sessionID, runID string) ([]corechat.ToolResult, error)
}

func TerminalConversation(
	ctx context.Context,
	publications ToolResultPublications,
	sessionID, runID string,
	messages []corechat.Message,
	outcome run.Outcome,
	detail string,
) ([]corechat.Message, error) {
	history, err := conversation.New(messages)
	if err != nil {
		return nil, err
	}
	if !history.HasOpenToolCalls() {
		return nil, nil
	}
	completed, err := publications.UnpublishedToolResults(ctx, sessionID, runID)
	if err != nil {
		return nil, fmt.Errorf("runs: read exact Tool results for terminal Run %q: %w", runID, err)
	}
	_, appended, err := history.CloseOpenToolCallsWithResults(TerminalToolResult(outcome, detail), completed)
	return appended, err
}

// TerminalToolResult is the text an open tool call carries once its Run ended
// without producing one. Every path that closes a parked or recovered tool call
// takes it from here: the text reaches the model as the call's result, so two
// spellings would make the same ended Run read as two different histories.
func TerminalToolResult(outcome run.Outcome, detail string) string {
	var result string
	switch outcome {
	case run.OutcomeCanceled:
		result = "tool call canceled before completion"
	case run.OutcomeTimedOut:
		result = "tool call did not complete before the run timed out"
	case run.OutcomeLost:
		result = lostToolResult
	case run.OutcomeFailed:
		result = "tool call did not complete because the run failed"
	default:
		result = "tool call ended without a result before the run finished"
	}
	if detail = strings.TrimSpace(detail); detail != "" && outcome == run.OutcomeCanceled {
		result += ": " + detail
	}
	return result
}
