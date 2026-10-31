package schedule

import (
	"errors"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
)

var ErrNotFound = errors.New("schedule: not found")

var ErrRevisionConflict = errors.New("schedule: revision conflict")

// Wrapping the final exact revision would make CAS identities ambiguous.
var errRevisionExhausted = errors.New("schedule: revision exhausted")

var (
	errIDRequired           = errors.New("schedule: id is required")
	ErrRevisionRequired     = errors.New("schedule: expected revision is required")
	ErrInstructionsRequired = errors.New("schedule: instructions is required")
	errCronRequired         = errors.New("schedule: cron is required")
	ErrInvalidCron          = errors.New("schedule: invalid cron")
)

type Draft struct {
	Title          string
	Instructions   string
	CWD            string
	ModelSelection modelchoice.Selection
	Cron           string
	TimeZone       string
	Enabled        bool
}

func (d Draft) Validate() error {
	if err := validateInstructions(d.Instructions); err != nil {
		return err
	}
	_, err := NewRecurrence(d.Cron, d.TimeZone)
	return err
}

type Snapshot struct {
	ID             string
	Title          string
	Instructions   string
	CWD            string
	ModelSelection modelchoice.Selection
	Cron           string
	TimeZone       string
	LastRunAt      time.Time `json:",omitzero"`
	NextRunAt      time.Time `json:",omitzero"`
	CreatedAt      time.Time
	Revision       uint64
}

type Schedule struct {
	id             scheduleIdentity
	title          string
	instructions   string
	cwd            string
	modelSelection modelchoice.Selection
	recurrence     Recurrence
	lastRunAt      time.Time
	// The cursor also determines enablement so a second enabled flag cannot drift from scheduling.
	nextRunAt time.Time
	createdAt time.Time
	revision  exactint.Counter
}

type Patch struct {
	Title        *string
	Instructions *string
	CWD          *string
	Selection    *modelchoice.Selection
	Cron         *string
	TimeZone     *string
	Enabled      *bool
}

func New(id string, draft Draft, createdAt time.Time) (Schedule, error) {
	parsedID, err := parseScheduleID(id)
	if err != nil {
		return Schedule{}, err
	}
	if err := validateInstructions(draft.Instructions); err != nil {
		return Schedule{}, err
	}
	recurrence, err := NewRecurrence(draft.Cron, draft.TimeZone)
	if err != nil {
		return Schedule{}, err
	}
	createdAt = canonicalTime(createdAt)
	value := Schedule{
		id: parsedID, title: draft.Title, instructions: draft.Instructions, cwd: draft.CWD,
		modelSelection: draft.ModelSelection, recurrence: recurrence,
		createdAt: createdAt, revision: exactint.First(),
	}
	return value.scheduledAfter(draft.Enabled, createdAt)
}

func Restore(snapshot Snapshot) (Schedule, error) {
	parsedID, err := parseScheduleID(snapshot.ID)
	if err != nil {
		return Schedule{}, err
	}
	revision, err := exactint.Restore(snapshot.Revision)
	if err != nil {
		return Schedule{}, errRevisionExhausted
	}
	recurrence, err := NewRecurrence(snapshot.Cron, snapshot.TimeZone)
	if err != nil {
		return Schedule{}, err
	}
	value := Schedule{
		id: parsedID, title: snapshot.Title, instructions: snapshot.Instructions,
		cwd: snapshot.CWD, modelSelection: snapshot.ModelSelection, recurrence: recurrence,
		lastRunAt: canonicalTime(snapshot.LastRunAt),
		nextRunAt: canonicalTime(snapshot.NextRunAt), createdAt: canonicalTime(snapshot.CreatedAt),
		revision: revision,
	}
	if err := value.Validate(); err != nil {
		return Schedule{}, err
	}
	return value, nil
}

func (s Schedule) Edit(p Patch, expectedRevision uint64, after time.Time) (Schedule, error) {
	if err := s.id.validate(); err != nil {
		return Schedule{}, err
	}
	if expectedRevision == 0 {
		return Schedule{}, ErrRevisionRequired
	}
	if expectedRevision != s.revision.Value() {
		return Schedule{}, ErrRevisionConflict
	}
	recurrenceChanged := (p.Cron != nil && *p.Cron != s.recurrence.Cron()) ||
		(p.TimeZone != nil && *p.TimeZone != s.recurrence.TimeZone())
	if p.Title != nil {
		s.title = *p.Title
	}
	if p.Instructions != nil {
		s.instructions = *p.Instructions
	}
	if p.CWD != nil {
		s.cwd = *p.CWD
	}
	if p.Selection != nil {
		s.modelSelection = *p.Selection
	}
	if recurrenceChanged {
		cron, timeZone := s.recurrence.Cron(), s.recurrence.TimeZone()
		if p.Cron != nil {
			cron = *p.Cron
		}
		if p.TimeZone != nil {
			timeZone = *p.TimeZone
		}
		recurrence, err := NewRecurrence(cron, timeZone)
		if err != nil {
			return Schedule{}, err
		}
		s.recurrence = recurrence
	}
	enabled := s.Enabled()
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	next, err := s.advanceRevision()
	if err != nil {
		return Schedule{}, err
	}
	if enabled != s.Enabled() || (enabled && recurrenceChanged) {
		return next.scheduledAfter(enabled, after)
	}
	return next, next.Validate()
}

func (s Schedule) advanceRevision() (Schedule, error) {
	if s.revision.IsZero() {
		return Schedule{}, ErrRevisionRequired
	}
	next, err := s.revision.Next()
	if err != nil {
		return Schedule{}, errRevisionExhausted
	}
	s.revision = next
	return s, nil
}

func (s Schedule) Validate() error {
	if err := s.validateProduct(); err != nil {
		return err
	}
	if s.revision.IsZero() {
		return ErrRevisionRequired
	}
	if s.createdAt.IsZero() {
		return errors.New("schedule: creation time is required")
	}
	if !s.lastRunAt.IsZero() && s.lastRunAt.Before(s.createdAt) {
		return errors.New("schedule: last run precedes creation")
	}
	return nil
}

func (s Schedule) validateProduct() error {
	if _, err := parseScheduleID(s.id.String()); err != nil {
		return err
	}
	if err := validateInstructions(s.instructions); err != nil {
		return err
	}
	if s.recurrence.zone == nil {
		return errCronRequired
	}
	return nil
}

func validateInstructions(instructions string) error {
	if strings.TrimSpace(instructions) == "" {
		return ErrInstructionsRequired
	}
	return nil
}

func (s Schedule) scheduledAfter(enabled bool, after time.Time) (Schedule, error) {
	if err := s.validateProduct(); err != nil {
		return Schedule{}, err
	}
	if !enabled {
		s.nextRunAt = time.Time{}
		return s, s.Validate()
	}
	next, err := s.recurrence.next(after)
	if err != nil {
		return Schedule{}, err
	}
	s.nextRunAt = next
	return s, s.Validate()
}

func (s Schedule) Snapshot() Snapshot {
	return Snapshot{
		ID: s.id.String(), Title: s.title, Instructions: s.instructions, CWD: s.cwd,
		ModelSelection: s.modelSelection, Cron: s.recurrence.Cron(), TimeZone: s.recurrence.TimeZone(),
		LastRunAt: s.lastRunAt, NextRunAt: s.nextRunAt, CreatedAt: s.createdAt,
		Revision: s.revision.Value(),
	}
}

func (s Schedule) ID() string                            { return s.id.String() }
func (s Schedule) Title() string                         { return s.title }
func (s Schedule) Instructions() string                  { return s.instructions }
func (s Schedule) CWD() string                           { return s.cwd }
func (s Schedule) ModelSelection() modelchoice.Selection { return s.modelSelection }
func (s Schedule) Recurrence() Recurrence                { return s.recurrence }
func (s Schedule) Enabled() bool                         { return !s.nextRunAt.IsZero() }
func (s Schedule) LastRunAt() time.Time                  { return s.lastRunAt }
func (s Schedule) NextRunAt() time.Time                  { return s.nextRunAt }
func (s Schedule) CreatedAt() time.Time                  { return s.createdAt }
func (s Schedule) Revision() uint64                      { return s.revision.Value() }
