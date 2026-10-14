package execution

import (
	"context"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	corechat "github.com/Tangerg/scope/core/chat"
)

// PlanReader is the prompt composer's read-only view of the current Session
// Plan.
type PlanReader interface {
	List(ctx context.Context, sessionID string) ([]plan.Step, error)
}

// GoalReader is the working-context adapter's read-only view of the current
// autonomous Goal aggregate. A found Goal is valid and belongs to the requested
// Session.
type GoalReader interface {
	Current(ctx context.Context, sessionID string) (goal.Goal, bool, error)
}

// InteractionModelContextState supplies model-facing Session state whose value
// may change while an Interaction is running. It is deliberately separate from
// the frozen deployment instruction snapshot.
type InteractionModelContextState interface {
	CurrentSessionState(ctx context.Context, sessionID string) ([]corechat.Message, error)
}

// MemoryPromptItem contains only published values consumed by prompt composition.
// Feature lifecycle and business validation stay with the private backend.
type MemoryPromptItem struct {
	ID        string
	Content   string
	Pinned    bool
	UpdatedAt time.Time
}

// AgentMemoryReader supplies the pinned memory included in every fresh root.
type AgentMemoryReader interface {
	Items(ctx context.Context, scope string, project string) ([]MemoryPromptItem, error)
}

// AgentMemorySearcher supplies prompt-relevant non-pinned memory.
type AgentMemorySearcher interface {
	Search(ctx context.Context, project, query string, topK int) ([]MemoryPromptItem, error)
}
