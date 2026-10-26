package runs

import (
	"crypto/rand"
	"errors"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

const commitPrefix = "run_commit_"

var errInvalidCommit = errors.New("run commit identity must use the run_commit_ prefix and contain bounded URI-safe ASCII")

type CommitID struct {
	value string
}

func newRunCommitID() CommitID {
	return CommitID{value: commitPrefix + rand.Text()}
}

func ParseCommit(raw string) (CommitID, error) {
	if err := identity.ValidateURISafeASCII(raw, identity.MaximumResourceCharacters); err != nil {
		return CommitID{}, errInvalidCommit
	}
	if len(raw) <= len(commitPrefix) || !strings.HasPrefix(raw, commitPrefix) {
		return CommitID{}, errInvalidCommit
	}
	return CommitID{value: raw}, nil
}

// Validate proves that i is a constructed commit identity. The canonical form
// is established by [newRunCommitID] and [ParseCommit]; the zero value is reserved
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
