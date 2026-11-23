package authoring

import (
	"errors"
	"strconv"
)

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
