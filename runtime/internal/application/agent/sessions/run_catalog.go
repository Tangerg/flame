package sessions

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type runReader interface {
	ListRuns(ctx context.Context, sessionID identity.SessionID) ([]run.Run, error)
}
