package sessions

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

var ErrWorkspaceMutationPending = errors.New("sessions: workspace mutation is pending")

type WorkspaceRollbackSpec struct {
	SessionID      identity.SessionID
	ToRunID        identity.RunID
	RestoreHistory bool
	Receipt        WorkspaceRollbackReceipt
}

type WorkspaceRollbackReceipt struct {
	Key         string
	Fingerprint string
}

type WorkspaceMutation struct {
	WorkspaceRollbackSpec
	CWD string
}
