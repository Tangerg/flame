package execution

import (
	"context"
	"log/slog"
)

// RunMaintenance owns best-effort housekeeping after a successful Run commits
// and before the live Segment stream closes.
type RunMaintenance interface {
	Maintain(ctx context.Context, input RunMaintenanceInput) RunMaintenanceResult
}

// RunMaintenanceInput is the finished root Interaction's maintenance context.
type RunMaintenanceInput struct {
	RootMemberID string
	RootRunID    string
	SessionID    string
	// WorkspaceCWD is the Session's project directory. Mined skills and
	// consolidated memory are durable project knowledge, so they are addressed
	// by the workspace even when the Run that produced them executed in an
	// isolated copy that is about to be discarded.
	WorkspaceCWD string
}

// RunMaintenanceResult reports independent best-effort failures without
// rewriting the already-produced assistant response.
type RunMaintenanceResult struct {
	Errors []error
}

// interactionLifecycleHooks owns the Runtime lifecycle events that are not
// part of Tool authorization or prompt composition. These describe the Session
// rather than one Tool call, so their cwd is the Session's project directory:
// it is what a hook trust grant was given to, and it outlives the isolated copy
// an individual Run may have executed in.
type interactionLifecycleHooks interface {
	BeforeCompaction(ctx context.Context, sessionID, cwd string) (bool, error)
	NotifyWaiting(ctx context.Context, sessionID, cwd string) error
	NotifyStopped(ctx context.Context, sessionID, cwd, reason string) error
}

func (i *interactionSession) maintainCompletedRoot() {
	if i.maintenance == nil || i.start.SessionID == "" {
		return
	}
	result := i.maintenance.Maintain(i.lifetime.execution, RunMaintenanceInput{
		RootMemberID: i.rootID.String(),
		RootRunID:    i.start.RunID,
		SessionID:    i.start.SessionID,
		WorkspaceCWD: i.start.WorkspaceCWD,
	})
	for _, err := range result.Errors {
		if err != nil {
			slog.ErrorContext(i.lifetime.execution, "execution: run maintenance failed",
				"session.id", i.start.SessionID, "error", err,
			)
		}
	}
}
