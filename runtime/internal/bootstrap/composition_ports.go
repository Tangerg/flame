package bootstrap

import (
	"context"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

// TerminalResource is a process-owned adapter whose Close call is one-shot:
// once Close returns, the resource has reached its final state even when it
// reports a diagnostic. Host bounds and joins the call itself, so adapters do
// not need a second timeout or retry layer.
type TerminalResource interface {
	Close() error
}

// PlanStore is the composition-root union shared by prompt assembly, Plan use cases,
// the plan.updated projection, the plan.get read, and session lifecycle
// cleanup. Boundary is the run-boundary history rollback and fork restore from;
// capturing a boundary is not here, because no consumer asks for it — a Run
// reaching terminal is what records one.
type PlanStore interface {
	List(ctx context.Context, sessionID string) ([]plan.Step, error)
	State(ctx context.Context, sessionID string) (plan.Current, error)
	Save(ctx context.Context, sessionID string, expected plan.Version, replacement plan.State) error
	Boundary(ctx context.Context, runID string) ([]plan.Step, bool, error)
	DeleteSession(ctx context.Context, sessionID string) error
}

// ApprovalRuleStore is the composition-root union shared by the domain policy
// evaluator and session lifecycle cleanup.
type ApprovalRuleStore interface {
	approvals.RuleStore
	DeleteSession(ctx context.Context, sessionID string) error
}

// ScheduleStore is the composition-root union shared by schedule management,
// firing, and Run opening effects. The consumers retain their narrower
// application-owned ports.
type ScheduleStore interface {
	ListPage(ctx context.Context, afterCreatedAt time.Time, afterID string, limit int) ([]schedule.Schedule, error)
	Get(ctx context.Context, id string) (schedule.Schedule, error)
	Insert(ctx context.Context, sc schedule.Schedule) error
	Update(ctx context.Context, sc schedule.Schedule, expectedRevision uint64) (schedule.Schedule, error)
	Delete(ctx context.Context, id string) (bool, error)
	Due(ctx context.Context, now time.Time, limit int) ([]schedule.Schedule, error)
	Claim(ctx context.Context, claim schedule.Claim) (bool, error)
	Pending(ctx context.Context, limit int) ([]schedule.Occurrence, error)
	Accept(ctx context.Context, acceptance schedule.Acceptance) error
	RecordRun(ctx context.Context, record schedule.RunRecord) error
}

// HookResolver is the runtime's consumer view of lifecycle-hook resolution.
type HookResolver interface {
	For(ctx context.Context, cwd string) (*apphooks.Bound, error)
	Inspect(ctx context.Context, cwd string) (apphooks.Inspection, error)
}

// Transactor runs fn inside a single storage transaction; the seam the
// composition root uses to give the Runtime cross-store atomicity without
// coupling it to the sqlite backend.
type Transactor func(ctx context.Context, fn func(context.Context) error) error

// UtilityRoleStore persists the global utility-model role across restarts. The
// composition root loads it at boot to seed the live cell and injects the
// sqlite-backed implementation. A nil store disables persistence — the role
// stays in-process only. Consumed by bootstrap + the capabilities coordinator.
type UtilityRoleStore interface {
	LoadUtilityRole(ctx context.Context) (modelref.Selection, error)
	SaveUtilityRole(ctx context.Context, role modelref.Selection) error
}

// EmbeddingRoleStore persists the embedding-model role across restarts. nil
// disables persistence — the role stays whatever was last set in-process.
type EmbeddingRoleStore interface {
	LoadEmbeddingRole(ctx context.Context) (modelref.Selection, error)
	SaveEmbeddingRole(ctx context.Context, role modelref.Selection) error
}
