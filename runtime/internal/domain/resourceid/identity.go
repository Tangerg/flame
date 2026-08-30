// Package resourceid owns exact opaque identities for Flame's durable runtime
// resources. Values are compared and projected verbatim; construction never
// trims, case-folds, normalizes, or otherwise repairs caller material.
package resourceid

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/resourceidentity"
)

type value struct {
	text string
}

func parse(kind, text string, maximumCharacters int) (value, error) {
	if text == "" {
		return value{}, fmt.Errorf("%s identity is empty", kind)
	}
	if !utf8.ValidString(text) {
		return value{}, fmt.Errorf("%s identity is not valid UTF-8", kind)
	}
	if characters := utf8.RuneCountInString(text); characters > maximumCharacters {
		return value{}, fmt.Errorf("%s identity has %d characters, maximum is %d", kind, characters, maximumCharacters)
	}
	for _, character := range text {
		if unicode.IsSpace(character) || !unicode.IsPrint(character) {
			return value{}, fmt.Errorf("%s identity contains whitespace or a non-printing character", kind)
		}
	}
	return value{text: text}, nil
}

// SessionID is one exact durable Session identity.
type SessionID struct{ value }

func ParseSession(text string) (SessionID, error) {
	parsed, err := parse("session", text, resourceidentity.MaximumCharacters)
	return SessionID{value: parsed}, err
}

func (i SessionID) String() string { return i.text }
func (i SessionID) Validate() error {
	_, err := ParseSession(i.text)
	return err
}

// RunID is one exact logical Run identity.
type RunID struct{ value }

func ParseRun(text string) (RunID, error) {
	parsed, err := parse("run", text, resourceidentity.MaximumCharacters)
	return RunID{value: parsed}, err
}

func (i RunID) String() string { return i.text }
func (i RunID) Validate() error {
	_, err := ParseRun(i.text)
	return err
}

// SegmentID is one exact execution-generation identity.
type SegmentID struct{ value }

func ParseSegment(text string) (SegmentID, error) {
	parsed, err := parse("segment", text, resourceidentity.MaximumCharacters)
	return SegmentID{value: parsed}, err
}

func (i SegmentID) String() string { return i.text }
func (i SegmentID) Validate() error {
	_, err := ParseSegment(i.text)
	return err
}

// ItemID is one exact transcript or interrupt identity owned by a Run.
type ItemID struct{ value }

func ParseItem(text string) (ItemID, error) {
	parsed, err := parse("item", text, resourceidentity.MaximumCharacters)
	return ItemID{value: parsed}, err
}

func (i ItemID) String() string { return i.text }
func (i ItemID) Validate() error {
	_, err := ParseItem(i.text)
	return err
}

// ScheduleID is one exact durable scheduled-work identity.
type ScheduleID struct{ value }

func ParseSchedule(text string) (ScheduleID, error) {
	parsed, err := parse("schedule", text, resourceidentity.MaximumCharacters)
	return ScheduleID{value: parsed}, err
}

func (i ScheduleID) String() string { return i.text }
func (i ScheduleID) Validate() error {
	_, err := ParseSchedule(i.text)
	return err
}

// EventID is one exact replay identity. Its envelope is cursor-sized because
// the opaque token may carry a complete resumable journal position.
type EventID struct{ value }

func ParseEvent(text string) (EventID, error) {
	parsed, err := parse("event", text, resourceidentity.MaximumEventCharacters)
	return EventID{value: parsed}, err
}

func (i EventID) String() string { return i.text }
func (i EventID) Validate() error {
	_, err := ParseEvent(i.text)
	return err
}
