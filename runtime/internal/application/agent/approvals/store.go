package approvals

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
)

type RuleStore interface {
	Put(ctx context.Context, r approval.Rule) error

	Visible(ctx context.Context, sessionID identity.SessionID, workspaceDir string, limit int) ([]approval.Rule, error)

	Delete(ctx context.Context, id string) error
}
