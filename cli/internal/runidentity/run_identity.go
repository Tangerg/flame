// Package runidentity owns exact opaque Flame Runtime execution identities.
package runidentity

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

const (
	// MaximumCharacters is the CLI domain envelope for ordinary opaque Runtime
	// execution identities. The runtime adapter proves it remains synchronized
	// with the public protocol contract.
	MaximumCharacters = 256
	// MaximumEventCharacters is the separate envelope for a replay Event
	// identity, whose opaque value may contain a complete bounded cursor.
	MaximumEventCharacters = 65_540
)

// RunID is one exact logical Run identity.
type RunID struct{ value string }

// ParseRun admits a Run identity without normalizing it.
func ParseRun(value string) (RunID, error) {
	if err := validateOpaque("run id", value, MaximumCharacters); err != nil {
		return RunID{}, err
	}
	return RunID{value: value}, nil
}

func (r RunID) String() string { return r.value }

func (r RunID) Validate() error {
	_, err := ParseRun(r.value)
	return err
}

// SegmentID is one exact execution-segment generation identity.
type SegmentID struct{ value string }

// ParseSegment admits a Segment identity without normalizing it.
func ParseSegment(value string) (SegmentID, error) {
	if err := validateOpaque("segment id", value, MaximumCharacters); err != nil {
		return SegmentID{}, err
	}
	return SegmentID{value: value}, nil
}

func (s SegmentID) String() string { return s.value }

func (s SegmentID) Validate() error {
	_, err := ParseSegment(s.value)
	return err
}

// ItemID is one exact transcript or interrupt item identity owned by a Run.
type ItemID struct{ value string }

// ParseItem admits an Item identity without normalizing it.
func ParseItem(value string) (ItemID, error) {
	if err := validateOpaque("item id", value, MaximumCharacters); err != nil {
		return ItemID{}, err
	}
	return ItemID{value: value}, nil
}

func (i ItemID) String() string { return i.value }

func (i ItemID) Validate() error {
	_, err := ParseItem(i.value)
	return err
}

// EventID is one exact replay token within a stream segment.
type EventID struct{ value string }

// ParseEvent admits an Event identity without normalizing it.
func ParseEvent(value string) (EventID, error) {
	if err := validateOpaque("event id", value, MaximumEventCharacters); err != nil {
		return EventID{}, err
	}
	return EventID{value: value}, nil
}

func (e EventID) String() string { return e.value }

func (e EventID) Validate() error {
	_, err := ParseEvent(e.value)
	return err
}

func validateOpaque(name, value string, maximumCharacters int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s is not valid UTF-8", name)
	}
	if value == "" {
		return fmt.Errorf("%s is empty", name)
	}
	if characters := utf8.RuneCountInString(value); characters > maximumCharacters {
		return fmt.Errorf("%s has %d characters, maximum is %d", name, characters, maximumCharacters)
	}
	for _, character := range value {
		if unicode.IsSpace(character) || !unicode.IsPrint(character) {
			return fmt.Errorf("%s contains whitespace or a non-printing character", name)
		}
	}
	return nil
}
