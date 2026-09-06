package agentexec

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"
)

// RunMaintenance owns best-effort housekeeping after a clean Interaction but
// before its terminal fact closes the live Segment stream.
type RunMaintenance interface {
	Maintain(ctx context.Context, input RunMaintenanceInput) RunMaintenanceResult
}

// RunMaintenanceInput is the finished root Interaction's maintenance context.
type RunMaintenanceInput struct {
	SessionID               string
	CWD                     string
	ToolCalls               int
	DurableContextCompacted bool
}

// RunMaintenanceResult reports independent best-effort failures without
// rewriting the already-produced assistant response.
type RunMaintenanceResult struct {
	Errors []error
}

// InteractionLifecycleHooks owns the Runtime lifecycle events that are not
// part of Tool authorization or prompt composition.
type InteractionLifecycleHooks interface {
	BeforeCompaction(ctx context.Context, sessionID, cwd string) (bool, error)
	NotifyWaiting(ctx context.Context, sessionID, cwd string) error
	NotifyStopped(ctx context.Context, sessionID, cwd, reason string) error
}

func (i *interactionSession) maintainCompletedRoot() {
	if i.maintenance == nil || i.start.SessionID == "" {
		return
	}
	toolCalls := i.accounting.toolCallCount()
	result := i.maintenance.Maintain(i.lifetime.execution, RunMaintenanceInput{
		SessionID:               i.start.SessionID,
		CWD:                     i.start.CWD,
		ToolCalls:               toolCalls,
		DurableContextCompacted: i.state.durableContextCompacted(),
	})
	for _, err := range result.Errors {
		if err != nil {
			trace.SpanFromContext(i.lifetime.execution).RecordError(
				fmt.Errorf("agentexec: Run maintenance: %w", err),
			)
		}
	}
}
