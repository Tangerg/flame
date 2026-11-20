package goals

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/samber/lo"
)

// WithInvalidations returns store wrapped so that every goal write that actually
// landed publishes its session's goal invalidation.
//
// The wrapper exists because a goal changes from many places — the lifecycle
// commands, each autonomous Run's settlement, the model's reported outcome, the
// boot reconcile — and all of them go through these two writes. Publishing at the
// write is what makes "a client is told whenever a goal moves" true by construction
// instead of by every caller remembering; it is also why the 4-second poll this
// replaces could be deleted rather than merely lengthened.
//
// A store write here is its own transaction, so returning means committed. The
// session delete/rollback cascade clears goals inside a session write-set instead,
// and that transaction publishes its own notices — one commit, one author, no
// double signal.
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
