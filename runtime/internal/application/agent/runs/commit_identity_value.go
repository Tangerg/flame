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

func (i CommitID) Validate() error {
	if i.IsZero() {
		return errInvalidCommit
	}
	return nil
}

func (i CommitID) IsZero() bool { return i.value == "" }

func (i CommitID) String() string { return i.value }
