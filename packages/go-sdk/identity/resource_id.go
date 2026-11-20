// Identities are compared and projected verbatim; construction never
// trims, case-folds, normalizes, or otherwise repairs caller material.
package identity

import (
	"database/sql/driver"
	"fmt"
	"regexp"
	"strings"

	framehost "github.com/Tangerg/frame/host"
)

type value struct {
	text string
}

// requireConstructed reports whether an identity came from parsing. Exact text,
// length and character rules are established there, so the only identity that
// can reach a caller without them is the zero one.
func requireConstructed(kind, text string) error {
	if text == "" {
		return fmt.Errorf("%s identity is empty", kind)
	}
	return nil
}

// kind is the grammar one identity type is parsed against. Its types are
// unexported, so no other package can mint an identity kind.
type kind interface{ name() string }

type (
	sessionKind struct{}
	runKind     struct{}
	segmentKind struct{}
	itemKind    struct{}
)

func (sessionKind) name() string { return "session" }
func (runKind) name() string     { return "run" }
func (segmentKind) name() string { return "segment" }
func (itemKind) name() string    { return "item" }

// ID is an opaque Runtime identity of one kind. Only parsing constructs a
// nonzero value, so a held ID satisfies its grammar; the zero value means none.
// Distinct kinds keep otherwise identical wire strings from being interchanged.
type ID[K kind] struct{ text string }

type (
	SessionID = ID[sessionKind]
	RunID     = ID[runKind]
	SegmentID = ID[segmentKind]
	ItemID    = ID[itemKind]
)

func parseID[K kind](text string) (ID[K], error) {
	var k K
	if err := ValidateResource(k.name(), text, MaximumResourceCharacters); err != nil {
		return ID[K]{}, err
	}
	return ID[K]{text: text}, nil
}

func parseOptionalID[K kind](text string) (ID[K], error) {
	if text == "" {
		return ID[K]{}, nil
	}
	return parseID[K](text)
}

func ParseSession(text string) (SessionID, error) { return parseID[sessionKind](text) }
func ParseRun(text string) (RunID, error)         { return parseID[runKind](text) }
func ParseSegment(text string) (SegmentID, error) { return parseID[segmentKind](text) }
func ParseItem(text string) (ItemID, error)       { return parseID[itemKind](text) }

// The optional parsers read text where empty means no identity.
func ParseOptionalSession(text string) (SessionID, error) { return parseOptionalID[sessionKind](text) }
func ParseOptionalRun(text string) (RunID, error)         { return parseOptionalID[runKind](text) }
func ParseOptionalSegment(text string) (SegmentID, error) { return parseOptionalID[segmentKind](text) }
func ParseOptionalItem(text string) (ItemID, error)       { return parseOptionalID[itemKind](text) }

func (id ID[K]) String() string { return id.text }

func (id ID[K]) IsZero() bool { return id.text == "" }

// Compare orders identities by their text, the order every listing uses.
func (id ID[K]) Compare(other ID[K]) int { return strings.Compare(id.text, other.text) }

// Validate reports a missing identity; a present one was parsed.
func (id ID[K]) Validate() error {
	var k K
	return requireConstructed(k.name(), id.text)
}

func (id ID[K]) MarshalText() ([]byte, error) { return []byte(id.text), nil }

// UnmarshalText parses decoded text; empty text is the zero value.
func (id *ID[K]) UnmarshalText(text []byte) error {
	parsed, err := parseOptionalID[K](string(text))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// Value stores the identity as its text; the zero value stores empty text.
func (id ID[K]) Value() (driver.Value, error) { return id.text, nil }

// Scan parses a stored identity; NULL and empty text are the zero value.
func (id *ID[K]) Scan(source any) error {
	switch stored := source.(type) {
	case nil:
		*id = ID[K]{}
		return nil
	case string:
		return id.UnmarshalText([]byte(stored))
	case []byte:
		return id.UnmarshalText(stored)
	default:
		var k K
		return fmt.Errorf("%s identity cannot be scanned from %T", k.name(), source)
	}
}

func ValidateSession(text string) error { _, err := ParseSession(text); return err }
func ValidateRun(text string) error     { _, err := ParseRun(text); return err }

func ValidateSegment(text string) error { _, err := ParseSegment(text); return err }
func ValidateItem(text string) error    { _, err := ParseItem(text); return err }

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
func ParseHostInstallation(id framehost.InstallationID) (InstallationID, error) {
	return ParseInstallation(string(id))
}

func (i InstallationID) String() string  { return i.text }
func (i InstallationID) Validate() error { return requireConstructed("installation", i.text) }

// Host is the plugin host's key for this installation; the host accepts any
// name, and Flame keys every installation by its canonical UUID.
func (i InstallationID) Host() framehost.InstallationID { return framehost.InstallationID(i.text) }
