// Package schedule is the scheduled-run domain: a Schedule fires saved instructions
// on a cron trigger as a headless run (no client present). A firing claims
// schedules whose time has come, starts a run, and records the occurrence.
//
// A Schedule stores the final instruction text, not a recipe reference — the
// scheduler is deliberately decoupled from any authoring source, so deleting or
// renaming that source cannot break a schedule.
package schedule

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/exactint"
)

// ErrNotFound is returned when a schedule lookup cannot find an id.
var ErrNotFound = errors.New("schedule: not found")

// ErrRevisionConflict reports that a conditional update targeted a stale
// version of the schedule.
var ErrRevisionConflict = errors.New("schedule: revision conflict")

// ErrRevisionExhausted reports that a stored Schedule has consumed the final
// exact revision and cannot be mutated without making client CAS identities
// ambiguous.
var ErrRevisionExhausted = errors.New("schedule: revision exhausted")

// Validation sentinels returned by [Schedule.Validate]; callers can classify
// invalid fields without parsing error text.
var (
	// ErrIDRequired — an update target must identify a stored schedule.
	ErrIDRequired = errors.New("schedule: id is required")
	// ErrRevisionRequired — an external update must carry the version it read.
	ErrRevisionRequired = errors.New("schedule: expected revision is required")
	// ErrInstructionsRequired — a schedule with no instructions has nothing to fire.
	ErrInstructionsRequired = errors.New("schedule: instructions is required")
	// ErrCronRequired — a schedule with no cron has no trigger.
	ErrCronRequired = errors.New("schedule: cron is required")
	// ErrInvalidCron — the cron expression is not a supported five-field spec.
	ErrInvalidCron = errors.New("schedule: invalid cron")
)

// Draft is the admitted user-owned value from which a new Schedule is created.
// Identity, lifecycle time, cursor, and revision are assigned by [New].
type Draft struct {
	Title          string
	Instructions   string
	CWD            string
	ModelSelection modelref.Selection
	Cron           string
	Enabled        bool
}

// Validate checks user-owned schedule content before workspace resolution or
// saving begins.
func (d Draft) Validate() error {
	if err := d.ModelSelection.Validate(); err != nil {
		return fmt.Errorf("schedule: model selection: %w", err)
	}
	if err := validateInstructions(d.Instructions); err != nil {
		return err
	}
	if d.Cron == "" {
		return ErrCronRequired
	}
	return ValidateCron(d.Cron)
}

// Snapshot is the complete technical representation used by persistence.
type Snapshot struct {
	ID             string
	Title          string
	Instructions   string
	CWD            string
	ModelSelection modelref.Selection
	Cron           string
	Enabled        bool
	LastRunAt      time.Time
	NextRunAt      time.Time
	CreatedAt      time.Time
	Revision       uint64
}

// Schedule is saved instructions fired on a cron trigger. Its state is private:
// construction, edits, cursor changes, and revision advancement are aggregate
// behavior rather than mutable bags shared with coordinating callers and stores.
type Schedule struct {
	id             resourceid.ScheduleID
	title          string
	instructions   string
	cwd            string
	modelSelection modelref.Selection
	cron           string
	enabled        bool
	lastRunAt      time.Time
	nextRunAt      time.Time
	createdAt      time.Time
	revision       exactint.Counter
}

// Patch is a partial update to a Schedule. Nil fields keep the existing value;
// non-nil fields replace it, including replacing a string with "".
type Patch struct {
	Title        *string
	Instructions *string
	CWD          *string
	Selection    *modelref.Selection
	Cron         *string
	Enabled      *bool
}

// New creates revision one and derives the first cursor from the same canonical
// creation instant stored on the aggregate.
func New(id string, draft Draft, createdAt time.Time) (Schedule, error) {
	parsedID, err := parseScheduleID(id)
	if err != nil {
		return Schedule{}, err
	}
	if err := draft.Validate(); err != nil {
		return Schedule{}, err
	}
	createdAt = canonicalTime(createdAt)
	value := Schedule{
		id: parsedID, title: draft.Title, instructions: draft.Instructions, cwd: draft.CWD,
		modelSelection: draft.ModelSelection, cron: draft.Cron, enabled: draft.Enabled,
		createdAt: createdAt, revision: exactint.First(),
	}
	return value.ScheduledAfter(createdAt)
}

// Restore reconstructs a Schedule from durable state and rechecks every
// aggregate invariant.
func Restore(snapshot Snapshot) (Schedule, error) {
	parsedID, err := parseScheduleID(snapshot.ID)
	if err != nil {
		return Schedule{}, err
	}
	revision, err := exactint.Restore(snapshot.Revision)
	if err != nil {
		return Schedule{}, ErrRevisionExhausted
	}
	value := Schedule{
		id: parsedID, title: snapshot.Title, instructions: snapshot.Instructions,
		cwd: snapshot.CWD, modelSelection: snapshot.ModelSelection, cron: snapshot.Cron,
		enabled: snapshot.Enabled, lastRunAt: canonicalTime(snapshot.LastRunAt),
		nextRunAt: canonicalTime(snapshot.NextRunAt), createdAt: canonicalTime(snapshot.CreatedAt),
		revision: revision,
	}
	if err := value.Validate(); err != nil {
		return Schedule{}, err
	}
	return value, nil
}

// Edit applies one optimistic-concurrency checked replacement, derives its cron
// cursor from after, and advances its exact revision once. No caller can observe
// a replacement whose enabled state and cursor disagree.
func (s Schedule) Edit(p Patch, expectedRevision uint64, after time.Time) (Schedule, error) {
	if err := s.Validate(); err != nil {
		return Schedule{}, err
	}
	if expectedRevision == 0 {
		return Schedule{}, ErrRevisionRequired
	}
	if expectedRevision != s.revision.Value() {
		return Schedule{}, ErrRevisionConflict
	}
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
	if p.Cron != nil {
		s.cron = *p.Cron
	}
	if p.Enabled != nil {
		s.enabled = *p.Enabled
	}
	next, err := s.revision.Next()
	if err != nil {
		return Schedule{}, ErrRevisionExhausted
	}
	s.revision = next
	return s.ScheduledAfter(after)
}

// Validate checks every aggregate invariant before a Schedule crosses a
// boundary. The rule lives on the entity rather than only at an input edge.
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
	if s.enabled == s.nextRunAt.IsZero() {
		return errors.New("schedule: enabled state and next-run cursor disagree")
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
	if err := s.modelSelection.Validate(); err != nil {
		return fmt.Errorf("schedule: model selection: %w", err)
	}
	if err := validateInstructions(s.instructions); err != nil {
		return err
	}
	if s.cron == "" {
		return ErrCronRequired
	}
	if err := ValidateCron(s.cron); err != nil {
		return err
	}
	return nil
}

func validateInstructions(instructions string) error {
	if strings.TrimSpace(instructions) == "" {
		return ErrInstructionsRequired
	}
	return nil
}

// NextRevision returns the only legal successor of this stored Schedule's
// current revision. Persistence paths that mutate operational fields use this
// behavior instead of spelling arithmetic in SQL.
func (s Schedule) NextRevision() (uint64, error) {
	if err := s.Validate(); err != nil {
		return 0, err
	}
	next, err := s.revision.Next()
	if err != nil {
		return 0, ErrRevisionExhausted
	}
	return next.Value(), nil
}

// ScheduledAfter validates s and returns a copy with NextRunAt matching its
// enabled state. Disabled schedules always have a zero NextRunAt.
func (s Schedule) ScheduledAfter(after time.Time) (Schedule, error) {
	if err := s.validateProduct(); err != nil {
		return Schedule{}, err
	}
	if !s.enabled {
		s.nextRunAt = time.Time{}
		return s, s.Validate()
	}
	next, err := NextRun(s.cron, after)
	if err != nil {
		return Schedule{}, err
	}
	s.nextRunAt = next
	return s, s.Validate()
}

// Snapshot returns the complete owned persistence value.
func (s Schedule) Snapshot() Snapshot {
	return Snapshot{
		ID: s.id.String(), Title: s.title, Instructions: s.instructions, CWD: s.cwd,
		ModelSelection: s.modelSelection, Cron: s.cron, Enabled: s.enabled,
		LastRunAt: s.lastRunAt, NextRunAt: s.nextRunAt, CreatedAt: s.createdAt,
		Revision: s.revision.Value(),
	}
}

func (s Schedule) ID() string                         { return s.id.String() }
func (s Schedule) Title() string                      { return s.title }
func (s Schedule) Instructions() string               { return s.instructions }
func (s Schedule) CWD() string                        { return s.cwd }
func (s Schedule) ModelSelection() modelref.Selection { return s.modelSelection }
func (s Schedule) Cron() string                       { return s.cron }
func (s Schedule) Enabled() bool                      { return s.enabled }
func (s Schedule) LastRunAt() time.Time               { return s.lastRunAt }
func (s Schedule) NextRunAt() time.Time               { return s.nextRunAt }
func (s Schedule) CreatedAt() time.Time               { return s.createdAt }
func (s Schedule) Revision() uint64                   { return s.revision.Value() }
