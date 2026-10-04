// Package resourceid owns exact opaque identities for Flame's durable runtime
// resources. Values are compared and projected verbatim; construction never
// trims, case-folds, normalizes, or otherwise repairs caller material.
package resourceid

import (
	"fmt"
	"regexp"
	"strings"

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

var installationIDExpression = regexp.MustCompile(nonzeroUUIDPattern())

func InstallationIDPattern() string { return installationIDExpression.String() }

// Enumerating the first nonzero digit keeps the public pattern compatible with
// both RE2 and JavaScript while rejecting the nil UUID at the same boundary.
func nonzeroUUIDPattern() string {
	alternatives := make([]string, 0, 32)
	for first := range 32 {
		var pattern strings.Builder
		digit := 0
		for _, size := range []int{8, 4, 4, 4, 12} {
			if digit > 0 {
				pattern.WriteByte('-')
			}
			for range size {
				switch {
				case digit < first:
					pattern.WriteByte('0')
				case digit == first:
					pattern.WriteString("[1-9a-f]")
				default:
					pattern.WriteString("[0-9a-f]")
				}
				digit++
			}
		}
		alternatives = append(alternatives, pattern.String())
	}
	return "^(?:" + strings.Join(alternatives, "|") + ")$"
}

// InstallationID is the canonical UUID allocated to an installation, including
// its source-qualified tools and any persisted executable dependencies.
type InstallationID struct{ value }

func ParseInstallation(text string) (InstallationID, error) {
	if !installationIDExpression.MatchString(text) {
		return InstallationID{}, fmt.Errorf("installation identity must be a canonical nonzero UUID")
	}
	return InstallationID{value: value{text: text}}, nil
}
func (i InstallationID) String() string  { return i.text }
func (i InstallationID) Validate() error { return requireConstructed("installation", i.text) }
