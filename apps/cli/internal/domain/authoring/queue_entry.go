package authoring

import (
	"errors"
	"strconv"
)

// QueueEntryID is the stable, positive identity of one in-memory queue entry. Its
// zero value is invalid; absence is represented by map/pointer presence.
type QueueEntryID struct {
	value uint64
}

func (i QueueEntryID) Validate() error {
	if i.value == 0 {
		return errors.New("prompt queue entry id is not initialized")
	}
	return nil
}

func (i QueueEntryID) String() string { return strconv.FormatUint(i.value, 10) }
