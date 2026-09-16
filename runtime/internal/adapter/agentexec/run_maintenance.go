package agentexec

import (
	"context"
	"log/slog"
)

// RunMaintenance owns best-effort housekeeping after a clean Interaction but
// before its terminal fact closes the live Segment stream.
type RunMaintenance interface {
	Maintain(ctx context.Context, input RunMaintenanceInput) RunMaintenanceResult
}

// RunMaintenanceInput is the finished root Interaction's maintenance context.
type RunMaintenanceInput struct {
	SessionID string
	// WorkspaceCWD is the Session's project directory. Mined skills and
	// consolidated memory are durable project knowledge, so they are addressed
	// by the workspace even when the Run that produced them executed in an
	// isolated copy that is about to be discarded.
	WorkspaceCWD            string
	ToolCalls               int
	DurableContextCompacted bool
}

// RunMaintenanceResult reports independent best-effort failures without
// rewriting the already-produced assistant response.
type RunMaintenanceResult struct {
	Errors []error
}

// InteractionLifecycleHooks owns the Runtime lifecycle events that are not
// part of Tool authorization or prompt composition. These describe the Session
// rather than one Tool call, so their cwd is the Session's project directory:
// it is what a hook trust grant was given to, and it outlives the isolated copy
// an individual Run may have executed in.
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
		WorkspaceCWD:            i.start.WorkspaceCWD,
		ToolCalls:               toolCalls,
		DurableContextCompacted: i.state.durableContextCompacted(),
	})
	for _, err := range result.Errors {
		if err != nil {
			slog.ErrorContext(i.lifetime.execution, "agentexec: run maintenance failed",
				"session.id", i.start.SessionID, "error", err,
			)
		}
	}
}
