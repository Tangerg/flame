package replay

import (
	"errors"
)

// ErrCommandNotDispatched describes this attempt only. It cannot establish the
// outcome of an earlier attempt using the same command identity.
var ErrCommandNotDispatched = errors.New("command was not dispatched")

// ErrCommandOutcomeUnknown preserves a dispatched command when its reply cannot
// establish admission. It does not by itself authorize another transport attempt.
var ErrCommandOutcomeUnknown = errors.New("command acknowledgement is unknown")

var ErrCommandInputUnavailable = errors.New("prepared command input is unavailable")
