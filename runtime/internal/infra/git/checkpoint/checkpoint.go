// The shadow repo's GIT_DIR lives under the Flame home with the Session's cwd as
// its work tree, so the user's own .git is never touched. A fresh shadow borrows
// the source object store, then materializes every reachable object before the
// first boundary so source pruning cannot invalidate completed checkpoints.
package checkpoint

import (
	"errors"
)

var (
	ErrUnavailable = errors.New("checkpoint: unavailable")
	ErrConflict    = errors.New("checkpoint: restore conflicts with unarchived working tree material")
	// Git may already have changed part of the working tree, so callers must retain
	// their recovery intent.
	ErrRestoreIncomplete = errors.New("checkpoint: working tree restore may be incomplete")
	errSnapshotTooLarge  = errors.New("checkpoint: snapshot too large")
)
