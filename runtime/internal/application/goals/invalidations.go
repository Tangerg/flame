package goals

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/samber/lo"
)

// Goals change from many places, all through these two writes, so publishing at
// the write notifies clients of every move by construction. Session cascades clear
// goals inside their own write-set and publish their own notices.
func WithInvalidations(store Store, invalidations invalidation.Publish) (Store, error) {
	if lo.IsNil(store) || invalidations == nil {
		return nil, errors.New("goals: store and invalidations are required")
	}
	return notifyingStore{Store: store, invalidations: invalidations}, nil
}

type notifyingStore struct {
	Store
	invalidations invalidation.Publish
}

func (n notifyingStore) Save(ctx context.Context, replacement goal.Replacement) (bool, error) {
	applied, err := n.Store.Save(ctx, replacement)
	if err == nil && applied {
		n.publish(replacement.State().SessionID())
	}
	return applied, err
}

func (n notifyingStore) ClearIf(ctx context.Context, sessionID identity.SessionID, expected goal.Version) (bool, error) {
	applied, err := n.Store.ClearIf(ctx, sessionID, expected)
	if err == nil && applied {
		n.publish(sessionID)
	}
	return applied, err
}

func (n notifyingStore) publish(sessionID identity.SessionID) {
	n.invalidations.Notify(invalidation.InSession(invalidation.Goals, sessionID))
}
