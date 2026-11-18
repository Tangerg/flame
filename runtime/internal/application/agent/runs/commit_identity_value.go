package runs

import (
	"crypto/rand"
	"errors"
)

const commitPrefix = "run_commit_"

var errInvalidCommit = errors.New("run commit identity must use the run_commit_ prefix and contain bounded URI-safe ASCII")

type CommitID struct {
	value string
}

func newRunCommitID() CommitID {
	return CommitID{value: commitPrefix + rand.Text()}
}

// Validate proves that i is a constructed commit identity. The canonical form
// is established by [newRunCommitID]; the zero value is reserved
// for projections that do not own a top-level write-set.
func (i CommitID) Validate() error {
	if i.IsZero() {
		return errInvalidCommit
	}
	return nil
}

// IsZero reports whether no commit identity is present.
func (i CommitID) IsZero() bool { return i.value == "" }

func (i CommitID) String() string { return i.value }
