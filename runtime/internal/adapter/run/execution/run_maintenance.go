package execution

import (
	"context"
	"log/slog"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

type RunMaintenance interface {
	Maintain(ctx context.Context, input RunMaintenanceInput) RunMaintenanceResult
}

type RunMaintenanceInput struct {
	RootMemberID string
	RootRunID    identity.RunID
	SessionID    identity.SessionID
	WorkspaceCWD string
}

type RunMaintenanceResult struct {
	Errors []error
}

type interactionLifecycleHooks interface {
	BeforeCompaction(ctx context.Context, sessionID identity.SessionID, cwd string) (bool, error)
	NotifyWaiting(ctx context.Context, sessionID identity.SessionID, cwd string) error
	NotifyStopped(ctx context.Context, sessionID identity.SessionID, cwd, reason string) error
}

func (i *interactionSession) maintainCompletedRoot() {
	if i.start.SessionID.IsZero() {
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
				telemetry.SessionID(i.start.SessionID), telemetry.Error(err),
			)
		}
	}
}
