package bootstrap

import (
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/google/uuid"
)

func newSessionID() identity.SessionID {
	return mustIdentity(identity.NewSession(uuid.NewString()))
}

func newRunID() identity.RunID         { return mustIdentity(identity.NewRun(uuid.NewString())) }
func newSegmentID() identity.SegmentID { return mustIdentity(identity.NewSegment(uuid.NewString())) }

func mustIdentity[ID any](id ID, err error) ID {
	if err != nil {
		panic(err)
	}
	return id
}

func newItemID() identity.ItemID { return mustIdentity(identity.NewItem(uuid.NewString())) }
