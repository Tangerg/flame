// Package commitidentity owns the idempotency identity of one immutable
// top-level Runtime write-set.
package commitidentity

import (
	"crypto/rand"
	"errors"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/resourceidentity"
)

const Prefix = "run_commit_"

var ErrInvalid = errors.New("Run commit identity must use the run_commit_ prefix and contain bounded URI-safe ASCII")

type ID struct {
	value string
}

func New() ID {
	return ID{value: Prefix + rand.Text()}
}

func Parse(raw string) (ID, error) {
	if len(raw) <= len(Prefix) || len(raw) > resourceidentity.MaximumCharacters || !strings.HasPrefix(raw, Prefix) {
		return ID{}, ErrInvalid
	}
	for index := range len(raw) {
		character := raw[index]
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' || character == '.' || character == ':' {
			continue
		}
		return ID{}, ErrInvalid
	}
	return ID{value: raw}, nil
}

// Validate proves that i is a constructed, canonical commit identity. The zero
// value is reserved for projections that do not own a top-level write-set.
func (i ID) Validate() error {
	_, err := Parse(i.value)
	return err
}

// IsZero reports whether no commit identity is present.
func (i ID) IsZero() bool { return i.value == "" }

func (i ID) String() string { return i.value }
