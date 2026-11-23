package approvals

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

type SessionLookup interface {
	Get(ctx context.Context, id identity.SessionID) (session.Session, error)
}

type RuleChange struct {
	Tool      tool.Ref
	Scope     approval.Scope
	SessionID identity.SessionID
	Subject   approval.Subject
	Decision  approval.Decision
}
