// Identities are compared and projected verbatim; construction never
// trims, case-folds, normalizes, or otherwise repairs caller material.
package identity

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Tangerg/frame"
)

type value struct {
	text string
}

func parse(kind, text string, maximumCharacters int) (value, error) {
	if err := ValidateResource(kind, text, maximumCharacters); err != nil {
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

// Forked Runs and their transcript Items retain both identities together.
// Distinct types prevent interchanging two otherwise opaque wire strings.
type SessionID struct{ value }
type RunID struct{ value }

func ParseSession(text string) (SessionID, error) {
	v, err := parse("session", text, MaximumResourceCharacters)
	return SessionID{v}, err
}
func ParseRun(text string) (RunID, error) {
	v, err := parse("run", text, MaximumResourceCharacters)
	return RunID{v}, err
}
func (id SessionID) String() string     { return id.text }
func (id RunID) String() string         { return id.text }
func ValidateSession(text string) error { _, err := ParseSession(text); return err }
func ValidateRun(text string) error     { _, err := ParseRun(text); return err }

func ValidateSegment(text string) error {
	return ValidateResource("segment", text, MaximumResourceCharacters)
}

// ValidateItem has no Parse counterpart: a transcript or interrupt identity is
// only ever a field of the Item, Lineage or commit it belongs to.
func ValidateItem(text string) error {
	return ValidateResource("item", text, MaximumResourceCharacters)
}

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

// ParseHostInstallation reads the plugin host's key back as Flame's identity.
func ParseHostInstallation(id frame.InstallationID) (InstallationID, error) {
	return ParseInstallation(string(id))
}

func (i InstallationID) String() string  { return i.text }
func (i InstallationID) Validate() error { return requireConstructed("installation", i.text) }

// Host is the plugin host's key for this installation; the host accepts any
// name, and Flame keys every installation by its canonical UUID.
func (i InstallationID) Host() frame.InstallationID { return frame.InstallationID(i.text) }
