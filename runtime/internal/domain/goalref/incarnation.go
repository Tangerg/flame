// Package goalref owns exact references to durable Goal incarnations.
//
// An incarnation is a technical coordination identity shared by the Goal,
// its admitted root Run, and crash-recovery records. It is not a Session or
// Run resource identity, and callers must compare and persist it verbatim.
package goalref

import (
	"fmt"
	"unicode"
	"unicode/utf8"
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
	if text == "" {
		return IncarnationID{}, fmt.Errorf("goal incarnation identity is empty")
	}
	if !utf8.ValidString(text) {
		return IncarnationID{}, fmt.Errorf("goal incarnation identity is not valid UTF-8")
	}
	if characters := utf8.RuneCountInString(text); characters > MaximumIncarnationCharacters {
		return IncarnationID{}, fmt.Errorf(
			"goal incarnation identity has %d characters, maximum is %d",
			characters,
			MaximumIncarnationCharacters,
		)
	}
	for _, character := range text {
		if unicode.IsSpace(character) || !unicode.IsPrint(character) {
			return IncarnationID{}, fmt.Errorf(
				"goal incarnation identity contains whitespace or a non-printing character",
			)
		}
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

func (i IncarnationID) Validate() error {
	_, err := ParseIncarnation(i.text)
	return err
}
