package promptqueue

import (
	"errors"
	"strconv"
)

// EntryID is the stable, positive identity of one in-memory queue entry. Its
// zero value is invalid; absence is represented by map/pointer presence.
type EntryID struct {
	value uint64
}

func (i EntryID) Validate() error {
	if i.value == 0 {
		return errors.New("prompt queue entry id is not initialized")
	}
	return nil
}

func (i EntryID) String() string { return strconv.FormatUint(i.value, 10) }
