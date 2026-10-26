package identity

import (
	"crypto/rand"
	"errors"
	"strings"
)

const CommitPrefix = "run_commit_"

var ErrInvalidCommit = errors.New("run commit identity must use the run_commit_ prefix and contain bounded URI-safe ASCII")

type CommitID struct {
	value string
}

func NewCommit() CommitID {
	return CommitID{value: CommitPrefix + rand.Text()}
}

func ParseCommit(raw string) (CommitID, error) {
	if err := ValidateURISafeASCII(raw, MaximumResourceCharacters); err != nil {
		return CommitID{}, ErrInvalidCommit
	}
	if len(raw) <= len(CommitPrefix) || !strings.HasPrefix(raw, CommitPrefix) {
		return CommitID{}, ErrInvalidCommit
	}
	return CommitID{value: raw}, nil
}

// Validate proves that i is a constructed commit identity. The canonical form
// is established by [NewCommit] and [ParseCommit]; the zero value is reserved
// for projections that do not own a top-level write-set.
func (i CommitID) Validate() error {
	if i.IsZero() {
		return ErrInvalidCommit
	}
	return nil
}

// IsZero reports whether no commit identity is present.
func (i CommitID) IsZero() bool { return i.value == "" }

func (i CommitID) String() string { return i.value }
