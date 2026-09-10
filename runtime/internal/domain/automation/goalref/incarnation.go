// Package goalref owns exact references to durable Goal incarnations.
//
// An incarnation is a technical coordination identity shared by the Goal,
// its admitted root Run, and crash-recovery records. It is not a Session or
// Run resource identity, and callers must compare and persist it verbatim.
package goalref

import (
	"errors"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// MaximumIncarnationCharacters bounds every durable Goal-incarnation key.
// Current producers use UUIDs; the larger envelope leaves encoding headroom
// without permitting unbounded database keys or recovery payloads.
const MaximumIncarnationCharacters = 128

// IncarnationID is one exact durable Goal incarnation identity.
type IncarnationID struct {
	text string
}

// ParseIncarnation validates and preserves text exactly. It never trims,
// normalizes, case-folds, or otherwise repairs caller input.
func ParseIncarnation(text string) (IncarnationID, error) {
	if err := runtimeidentity.ValidateResource("goal incarnation", text, MaximumIncarnationCharacters); err != nil {
		return IncarnationID{}, err
	}
	return IncarnationID{text: text}, nil
}

// ParseOptionalIncarnation validates a field whose absence is meaningful.
func ParseOptionalIncarnation(text string) (IncarnationID, bool, error) {
	if text == "" {
		return IncarnationID{}, false, nil
	}
	parsed, err := ParseIncarnation(text)
	return parsed, err == nil, err
}

func (i IncarnationID) String() string { return i.text }

// Validate reports whether the incarnation was parsed. Its exact text is
// established there, so an unconstructed identity is all this can reject.
func (i IncarnationID) Validate() error {
	if i.text == "" {
		return errors.New("goal incarnation identity is empty")
	}
	return nil
}
