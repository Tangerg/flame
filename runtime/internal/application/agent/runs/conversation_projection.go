package runs

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	corechat "github.com/Tangerg/scope/core/chat"
)

type ToolResultPublications interface {
	UnpublishedToolResults(ctx context.Context, sessionID identity.SessionID, runID identity.RunID) ([]corechat.ToolResult, error)
}

func TerminalConversation(
	ctx context.Context,
	publications ToolResultPublications,
	sessionID identity.SessionID, runID identity.RunID,
	messages []corechat.Message,
	ending run.Ending,
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
	_, appended, err := history.CloseOpenToolCallsWithResults(terminalToolResult(ending), completed)
	return appended, err
}

func terminalToolResult(ending run.Ending) string {
	switch ending := ending.(type) {
	case run.Canceled:
		if ending.Detail != "" {
			return "tool call canceled before completion: " + ending.Detail
		}
		return "tool call canceled before completion"
	case run.TimedOut:
		return "tool call did not complete before the run timed out"
	case run.Lost:
		return lostToolResult
	case run.Failed:
		return "tool call did not complete because the run failed"
	default:
		return "tool call ended without a result before the run finished"
	}
}
