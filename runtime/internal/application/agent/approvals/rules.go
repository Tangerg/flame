// Package approvals owns the runtime tool-permission use cases: the approval
// stance (mode) and the persisted per-session/project/global approval rules.
package approvals

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// SessionLookup resolves the validated application-owned Session read so rule
// listing can scope rules to its project directory.
type SessionLookup interface {
	Get(ctx context.Context, id string) (session.Session, error)
}

type RuleChange struct {
	Tool      tool.Ref
	Scope     approval.Scope
	SessionID string
	Subject   approval.Subject
	Decision  approval.Decision
}
