package authoring

import "errors"

var (
	ErrEntryNotFound   = errors.New("prompt queue: entry not found")
	ErrMoveUnavailable = errors.New("prompt queue: move unavailable")
)

type QueueEntry struct {
	ID        QueueEntryID
	CommandID CommandID
	SessionID string
	Message   Message
	Options   RunOptions
	Held      bool
}

type QueueSnapshot struct {
	Entries     []QueueEntry
	Dispatching *QueueEntryID
}
