package agentexec

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/agent/strategy/interaction"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
)

func toolControlOutcome(err error) bool {
	return errors.Is(err, interaction.ErrHostFailure) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, interaction.ErrToolInputRequired)
}

// Runtime commits feedback before returning from the Tool boundary. Supplying a
// Scope Failure keeps that durable output authoritative instead of predicting
// the strategy's diagnostic formatting for an ordinary error.
func toolFeedback(call chat.ToolCall, output chat.ToolOutput, cause error) (*chat.ToolResult, error) {
	if toolControlOutcome(cause) {
		return nil, cause
	}
	if cause == nil {
		if err := output.Validate(); err != nil {
			cause = fmt.Errorf("tool %q returned invalid output: %w", call.Name, err)
		} else {
			return &chat.ToolResult{ID: call.ID, Name: call.Name, Output: output}, nil
		}
	}
	if errors.Is(cause, tool.ErrAuthorizationDenied) {
		// Authorization is a strategy control category, so its redacted response
		// must not disclose an adapter's diagnostic or partial output.
		output = chat.NewTextToolOutput(fmt.Sprintf("error: tool %q is not authorized", call.Name))
		return &chat.ToolResult{ID: call.ID, Name: call.Name, Output: output, IsError: true}, cause
	}
	if failure, ok := errors.AsType[*tool.Failure](cause); ok {
		output = failure.Output()
	} else {
		output = chat.NewTextToolOutput("error: " + executorDiagnostic(cause))
	}
	failure, err := tool.NewFailure(cause, output)
	if err != nil {
		return nil, interaction.HostFailure(fmt.Errorf("agentexec: prepare Tool feedback: %w", errors.Join(cause, err)))
	}
	return &chat.ToolResult{ID: call.ID, Name: call.Name, Output: output, IsError: true}, failure
}
