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
	if len(raw) <= len(CommitPrefix) || len(raw) > MaximumResourceCharacters || !strings.HasPrefix(raw, CommitPrefix) {
		return CommitID{}, ErrInvalidCommit
	}
	for index := range len(raw) {
		character := raw[index]
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' || character == '.' || character == ':' {
			continue
		}
		return CommitID{}, ErrInvalidCommit
	}
	return CommitID{value: raw}, nil
}

// Validate proves that i is a constructed, canonical commit identity. The zero
// value is reserved for projections that do not own a top-level write-set.
func (i CommitID) Validate() error {
	_, err := ParseCommit(i.value)
	return err
}

// IsZero reports whether no commit identity is present.
func (i CommitID) IsZero() bool { return i.value == "" }

func (i CommitID) String() string { return i.value }
