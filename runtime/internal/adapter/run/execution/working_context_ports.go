package execution

import (
	"context"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	corechat "github.com/Tangerg/scope/core/chat"
)

type planReader interface {
	List(ctx context.Context, sessionID identity.SessionID) ([]plan.Step, error)
}

type goalReader interface {
	Current(ctx context.Context, sessionID identity.SessionID) (goal.Goal, bool, error)
}

type interactionModelContextState interface {
	CurrentSessionState(ctx context.Context, sessionID identity.SessionID) ([]corechat.Message, error)
}

type MemoryPromptItem struct {
	ID        string
	Content   string
	Pinned    bool
	UpdatedAt time.Time
}

type agentMemoryReader interface {
	Items(ctx context.Context, scope memorycontract.ItemsInputScope, project string) ([]MemoryPromptItem, error)
}

type agentMemorySearcher interface {
	Search(ctx context.Context, project, query string, topK int) ([]MemoryPromptItem, error)
}
