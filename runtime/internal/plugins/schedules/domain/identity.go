package schedule

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

const IDPrefix = "sch_"

const MaximumIDCharacters = runtimeidentity.MaximumResourceCharacters

const (
	occurrenceIDSeparator      = ":"
	maximumInt64TextCharacters = len("-9223372036854775808")

	MaximumOccurrenceIDCharacters = MaximumIDCharacters + len(occurrenceIDSeparator) + maximumInt64TextCharacters
)

type scheduleIdentity struct{ text string }

func (i scheduleIdentity) String() string { return i.text }

func (i scheduleIdentity) Validate() error {
	if i.text == "" {
		return ErrIDRequired
	}
	return nil
}

func parseScheduleID(text string) (scheduleIdentity, error) {
	if err := runtimeidentity.ValidateResource("schedule", text, MaximumIDCharacters); err != nil {
		return scheduleIdentity{}, fmt.Errorf("schedule: %w", err)
	}
	if !strings.HasPrefix(text, IDPrefix) || utf8.RuneCountInString(text) == utf8.RuneCountInString(IDPrefix) {
		return scheduleIdentity{}, ErrIDRequired
	}
	return scheduleIdentity{text: text}, nil
}

func ValidateID(text string) error {
	_, err := parseScheduleID(text)
	return err
}

// Schedule identity and due cursor make retries name the same occurrence.
type occurrenceIdentity struct {
	text       string
	scheduleID scheduleIdentity
	dueMillis  int64
}

func newOccurrenceIdentity(scheduleID scheduleIdentity, dueAt time.Time) (occurrenceIdentity, error) {
	if _, err := parseScheduleID(scheduleID.String()); err != nil {
		return occurrenceIdentity{}, fmt.Errorf("schedule: occurrence: %w", err)
	}
	dueMillis := dueAt.UTC().UnixMilli()
	if dueAt.IsZero() {
		return occurrenceIdentity{}, errors.New("schedule: occurrence due time is required")
	}
	text := scheduleID.String() + occurrenceIDSeparator + strconv.FormatInt(dueMillis, 10)
	return occurrenceIdentity{text: text, scheduleID: scheduleID, dueMillis: dueMillis}, nil
}

func parseOccurrenceIdentity(text string) (occurrenceIdentity, error) {
	if err := runtimeidentity.ValidateResource("schedule: occurrence", text, MaximumOccurrenceIDCharacters); err != nil {
		return occurrenceIdentity{}, err
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
	if err != nil || strconv.FormatInt(dueMillis, 10) != rawDueMillis {
		return occurrenceIdentity{}, errors.New("schedule: occurrence identity has a non-canonical due-time suffix")
	}
	return occurrenceIdentity{text: text, scheduleID: scheduleID, dueMillis: dueMillis}, nil
}

func (i occurrenceIdentity) Validate() error {
	_, err := parseOccurrenceIdentity(i.text)
	return err
}

func (i occurrenceIdentity) String() string { return i.text }
