package schedule

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/resourceidentity"
)

// IDPrefix is the server-generated Schedule resource namespace.
const IDPrefix = resourceidentity.SchedulePrefix

// MaximumIDCharacters bounds public Schedule resource identities.
const MaximumIDCharacters = resourceidentity.MaximumCharacters

const (
	occurrenceIDSeparator      = ":"
	maximumInt64TextCharacters = len("-9223372036854775808")
	// MaximumOccurrenceIDCharacters bounds the deterministic Schedule-firing key:
	// one bounded Schedule identity, its separator, and a signed int64 timestamp.
	MaximumOccurrenceIDCharacters = MaximumIDCharacters + len(occurrenceIDSeparator) + maximumInt64TextCharacters
)

func parseScheduleID(text string) (resourceid.ScheduleID, error) {
	parsed, err := resourceid.ParseSchedule(text)
	if err != nil {
		return resourceid.ScheduleID{}, fmt.Errorf("schedule: %w", err)
	}
	if !strings.HasPrefix(text, IDPrefix) || utf8.RuneCountInString(text) == utf8.RuneCountInString(IDPrefix) {
		return resourceid.ScheduleID{}, ErrIDRequired
	}
	return parsed, nil
}

// ValidateID validates one public Schedule resource identity exactly.
func ValidateID(text string) error {
	_, err := parseScheduleID(text)
	return err
}

// occurrenceIdentity is one deterministic durable cron-firing identity. Its
// structure is owned here because the Schedule and due cursor together define
// occurrence uniqueness and recovery idempotency.
type occurrenceIdentity struct {
	text       string
	scheduleID resourceid.ScheduleID
	dueMillis  int64
}

func newOccurrenceIdentity(scheduleID resourceid.ScheduleID, dueAt time.Time) (occurrenceIdentity, error) {
	if _, err := parseScheduleID(scheduleID.String()); err != nil {
		return occurrenceIdentity{}, fmt.Errorf("schedule: occurrence: %w", err)
	}
	dueMillis := dueAt.UTC().UnixMilli()
	if dueAt.IsZero() || dueMillis <= 0 {
		return occurrenceIdentity{}, errors.New("schedule: occurrence due time must be after the Unix epoch")
	}
	text := scheduleID.String() + occurrenceIDSeparator + strconv.FormatInt(dueMillis, 10)
	return occurrenceIdentity{text: text, scheduleID: scheduleID, dueMillis: dueMillis}, nil
}

func parseOccurrenceIdentity(text string) (occurrenceIdentity, error) {
	if text == "" {
		return occurrenceIdentity{}, errors.New("schedule: occurrence identity is empty")
	}
	if !utf8.ValidString(text) {
		return occurrenceIdentity{}, errors.New("schedule: occurrence identity is not valid UTF-8")
	}
	if characters := utf8.RuneCountInString(text); characters > MaximumOccurrenceIDCharacters {
		return occurrenceIdentity{}, fmt.Errorf(
			"schedule: occurrence identity has %d characters, maximum is %d",
			characters,
			MaximumOccurrenceIDCharacters,
		)
	}
	for _, character := range text {
		if unicode.IsSpace(character) || !unicode.IsPrint(character) {
			return occurrenceIdentity{}, errors.New(
				"schedule: occurrence identity contains whitespace or a non-printing character",
			)
		}
	}
	separator := strings.LastIndex(text, occurrenceIDSeparator)
	if separator <= 0 || separator == len(text)-len(occurrenceIDSeparator) {
		return occurrenceIdentity{}, errors.New("schedule: occurrence identity has no due-time suffix")
	}
	scheduleID, err := parseScheduleID(text[:separator])
	if err != nil {
		return occurrenceIdentity{}, fmt.Errorf("schedule: occurrence: %w", err)
	}
	rawDueMillis := text[separator+len(occurrenceIDSeparator):]
	dueMillis, err := strconv.ParseInt(rawDueMillis, 10, 64)
	if err != nil || dueMillis <= 0 || strconv.FormatInt(dueMillis, 10) != rawDueMillis {
		return occurrenceIdentity{}, errors.New("schedule: occurrence identity has a non-canonical due-time suffix")
	}
	return occurrenceIdentity{text: text, scheduleID: scheduleID, dueMillis: dueMillis}, nil
}

// ValidateOccurrenceID validates one deterministic durable firing identity
// without manufacturing an Acceptance or another lifecycle value.
func ValidateOccurrenceID(text string) error {
	_, err := parseOccurrenceIdentity(text)
	return err
}

func (i occurrenceIdentity) Validate() error {
	_, err := parseOccurrenceIdentity(i.text)
	return err
}

func (i occurrenceIdentity) String() string { return i.text }
