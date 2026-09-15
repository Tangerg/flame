package sessions

import "errors"

// ErrWorkspaceMutationPending reports that an unfinished file rollback still
// owns the Session or the working tree a new rollback addresses.
var ErrWorkspaceMutationPending = errors.New("sessions: workspace mutation is pending")

// WorkspaceMutation is the recorded intent of an in-flight file rollback. A Git
// reset updates multiple paths and is not atomic; a files+history rollback also
// spans two resources that cannot share an ACID transaction. The intent is
// therefore logged before the tree is touched and cleared only after every
// requested effect commits. A crash or incomplete reset is re-driven at boot.
//
// The whole value is the operation's identity, not just its key: recovery
// re-drives exactly this restore, so only this operation may clear it. SessionID
// and CWD are its ownership scope — one in-flight rollback per session, and one
// across the sessions sharing a working tree. ToRunID identifies the file
// boundary; RestoreHistory says whether recovery also applies the history cut.
type WorkspaceMutation struct {
	SessionID      string
	CWD            string
	ToRunID        string
	RestoreHistory bool
}
