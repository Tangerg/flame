package sessions

import "errors"

// ErrWorkspaceMutationPending reports that an unfinished file rollback still
// owns the Session or the working tree a new rollback addresses.
var ErrWorkspaceMutationPending = errors.New("sessions: workspace mutation is pending")

type WorkspaceRollbackSpec struct {
	SessionID      string
	ToRunID        string
	RestoreHistory bool
	Receipt        WorkspaceRollbackReceipt
}

type WorkspaceRollbackReceipt struct {
	Key         string
	Fingerprint string
}

// The intent owns the Session and working tree from before checkout until the
// history change and original receipt commit. The kept Run and its suffix
// remain available throughout recovery.
type WorkspaceMutation struct {
	WorkspaceRollbackSpec
	CWD string
}
