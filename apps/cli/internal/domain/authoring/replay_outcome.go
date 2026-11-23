package authoring

import (
	"errors"
)

var ErrCommandNotDispatched = errors.New("command was not dispatched")

var ErrCommandOutcomeUnknown = errors.New("command acknowledgement is unknown")

var ErrCommandInputUnavailable = errors.New("prepared command input is unavailable")
