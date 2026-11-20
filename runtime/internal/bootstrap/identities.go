package bootstrap

import (
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/google/uuid"
)

func newSessionID() identity.SessionID {
	return mustIdentity(identity.ParseSession(session.IDPrefix + uuid.NewString()))
}

// The generators panic only if a UUID stops satisfying an identity grammar,
// which no input can cause.
func newRunID() identity.RunID         { return mustIdentity(runs.NewRunID(uuid.NewString())) }
func newSegmentID() identity.SegmentID { return mustIdentity(runs.NewSegmentID(uuid.NewString())) }

func mustIdentity[ID any](id ID, err error) ID {
	if err != nil {
		panic(err)
	}
	return id
}
func newItemID() identity.ItemID { return mustIdentity(runs.NewItemID(uuid.NewString())) }
