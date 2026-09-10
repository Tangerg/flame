// Package resourceid owns exact opaque identities for Flame's durable runtime
// resources. Values are compared and projected verbatim; construction never
// trims, case-folds, normalizes, or otherwise repairs caller material.
package resourceid

import (
	"fmt"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

type value struct {
	text string
}

func parse(kind, text string, maximumCharacters int) (value, error) {
	if err := runtimeidentity.ValidateResource(kind, text, maximumCharacters); err != nil {
		return value{}, err
	}
	return value{text: text}, nil
}

// requireConstructed reports whether an identity came from [parse]. Exact text,
// length and character rules are established there, so the only identity that
// can reach a caller without them is the zero one.
func requireConstructed(kind, text string) error {
	if text == "" {
		return fmt.Errorf("%s identity is empty", kind)
	}
	return nil
}

// Most callers keep an identity as a field of a data value and need the rule
// rather than the identity type. Validate answers them directly; the Parse
// constructors below exist for the callers that carry the identity itself.
func ValidateSession(text string) error {
	return runtimeidentity.ValidateResource("session", text, runtimeidentity.MaximumResourceCharacters)
}

func ValidateRun(text string) error {
	return runtimeidentity.ValidateResource("run", text, runtimeidentity.MaximumResourceCharacters)
}

func ValidateSegment(text string) error {
	return runtimeidentity.ValidateResource("segment", text, runtimeidentity.MaximumResourceCharacters)
}

// ValidateItem has no Parse counterpart: a transcript or interrupt identity is
// only ever a field of the Item, Lineage or commit it belongs to.
func ValidateItem(text string) error {
	return runtimeidentity.ValidateResource("item", text, runtimeidentity.MaximumResourceCharacters)
}

// SessionID is one exact durable Session identity.
type SessionID struct{ value }

func ParseSession(text string) (SessionID, error) {
	parsed, err := parse("session", text, runtimeidentity.MaximumResourceCharacters)
	return SessionID{value: parsed}, err
}

func (i SessionID) String() string  { return i.text }
func (i SessionID) Validate() error { return requireConstructed("session", i.text) }

// RunID is one exact logical Run identity.
type RunID struct{ value }

func ParseRun(text string) (RunID, error) {
	parsed, err := parse("run", text, runtimeidentity.MaximumResourceCharacters)
	return RunID{value: parsed}, err
}

func (i RunID) String() string  { return i.text }
func (i RunID) Validate() error { return requireConstructed("run", i.text) }

// SegmentID is one exact execution-generation identity.
type SegmentID struct{ value }

func ParseSegment(text string) (SegmentID, error) {
	parsed, err := parse("segment", text, runtimeidentity.MaximumResourceCharacters)
	return SegmentID{value: parsed}, err
}

func (i SegmentID) String() string  { return i.text }
func (i SegmentID) Validate() error { return requireConstructed("segment", i.text) }

// ScheduleID is one exact durable scheduled-work identity.
type ScheduleID struct{ value }

func ParseSchedule(text string) (ScheduleID, error) {
	parsed, err := parse("schedule", text, runtimeidentity.MaximumResourceCharacters)
	return ScheduleID{value: parsed}, err
}

func (i ScheduleID) String() string  { return i.text }
func (i ScheduleID) Validate() error { return requireConstructed("schedule", i.text) }
