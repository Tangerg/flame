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

func requireConstructed(kind, text string) error {
	if text == "" {
		return fmt.Errorf("%s identity is empty", kind)
	}
	return nil
}

type kind interface{ name() string }

type (
	sessionKind struct{}
	runKind     struct{}
	segmentKind struct{}
	itemKind    struct{}
	projectKind struct{}
)

func (sessionKind) name() string { return "session" }
func (runKind) name() string     { return "run" }
func (segmentKind) name() string { return "segment" }
func (itemKind) name() string    { return "item" }
func (projectKind) name() string { return "project" }

type ID[K kind] struct{ text string }

type (
	SessionID = ID[sessionKind]
	RunID     = ID[runKind]
	SegmentID = ID[segmentKind]
	ItemID    = ID[itemKind]
	ProjectID = ID[projectKind]
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
func ParseProject(text string) (ProjectID, error) { return parseID[projectKind](text) }

func ParseOptionalSession(text string) (SessionID, error) { return parseOptionalID[sessionKind](text) }
func ParseOptionalRun(text string) (RunID, error)         { return parseOptionalID[runKind](text) }
func ParseOptionalSegment(text string) (SegmentID, error) { return parseOptionalID[segmentKind](text) }
func ParseOptionalItem(text string) (ItemID, error)       { return parseOptionalID[itemKind](text) }
func ParseOptionalProject(text string) (ProjectID, error) { return parseOptionalID[projectKind](text) }

func (id ID[K]) String() string { return id.text }

func (id ID[K]) IsZero() bool { return id.text == "" }

func (id ID[K]) Compare(other ID[K]) int { return strings.Compare(id.text, other.text) }

func (id ID[K]) Validate() error {
	var k K
	return requireConstructed(k.name(), id.text)
}

func (id ID[K]) MarshalText() ([]byte, error) { return []byte(id.text), nil }

func (id *ID[K]) UnmarshalText(text []byte) error {
	parsed, err := parseOptionalID[K](string(text))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

func (id ID[K]) Value() (driver.Value, error) { return id.text, nil }

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
func ValidateItem(text string) error    { _, err := ParseItem(text); return err }

var installationIDExpression = regexp.MustCompile(nonzeroUUIDPattern())

func InstallationIDPattern() string { return installationIDExpression.String() }

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

type InstallationID struct{ value }

func ParseInstallation(text string) (InstallationID, error) {
	if !installationIDExpression.MatchString(text) {
		return InstallationID{}, fmt.Errorf("installation identity must be a canonical nonzero UUID")
	}
	return InstallationID{value: value{text: text}}, nil
}

func ParseHostInstallation(id framehost.InstallationID) (InstallationID, error) {
	return ParseInstallation(string(id))
}

func (i InstallationID) String() string  { return i.text }
func (i InstallationID) Validate() error { return requireConstructed("installation", i.text) }

func (i InstallationID) Host() framehost.InstallationID { return framehost.InstallationID(i.text) }
