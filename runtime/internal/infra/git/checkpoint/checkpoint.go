// Package checkpoint snapshots a Session's working tree at Run boundaries via
// a per-Session/workspace SHADOW git repository, so a rollback can restore files
// (not just chat history) to a chosen Run without applying one project's tree
// to another project after Session relocation.
//
// The shadow repo's GIT_DIR lives under the Flame home, with the Session's cwd
// as its work tree — the user's own .git is never touched (git addresses the
// two independently, the classic dotfiles-repo pattern). Each Run boundary is
// anchored by a lightweight tag named for the Run id, so a restore checks out
// that tag. The only OS dependency is the git binary, which flame already
// requires for workspace diffs — so this is platform-agnostic.
//
// A fresh shadow may borrow the source object store while Git selects admitted
// workspace paths. Before publishing the first boundary, it materializes every
// reachable object so source pruning cannot invalidate completed checkpoints.
package checkpoint

import (
	"errors"
)

var (
	// ErrUnavailable means the requested boundary is absent or the workspace
	// overlaps checkpoint storage and cannot be snapshotted or restored safely.
	ErrUnavailable = errors.New("checkpoint: unavailable")
	// ErrConflict means restoring the target would overwrite material absent
	// from the pre-restore archive. The working tree has not been changed.
	ErrConflict = errors.New("checkpoint: restore conflicts with unarchived working tree material")
	// ErrRestoreIncomplete means checkout started but did not complete, so callers
	// must retain their recovery intent: Git may already have changed part of the
	// working tree even though the command returned an error.
	ErrRestoreIncomplete = errors.New("checkpoint: working tree restore may be incomplete")
	// ErrSnapshotTooLarge means the complete working-tree boundary cannot enter
	// the finite shadow-repository resource envelope.
	errSnapshotTooLarge = errors.New("checkpoint: snapshot too large")
)
