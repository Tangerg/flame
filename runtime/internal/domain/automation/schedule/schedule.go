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
	"time"

	"github.com/robfig/cron/v3"

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
	if d.Instructions == "" {
		return ErrInstructionsRequired
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

// Execution is the immutable instruction set captured by a firing. It is not a
// partial Schedule: lifecycle timestamps, cursor, and revision deliberately do
// not exist on this value.
type Execution struct {
	title          string
	instructions   string
	cwd            string
	modelSelection modelref.Selection
	cron           string
}

// ExecutionSnapshot is the persistence representation of [Execution].
type ExecutionSnapshot struct {
	Title          string
	Instructions   string
	CWD            string
	ModelSelection modelref.Selection
	Cron           string
}

// OccurrenceSnapshot is the complete durable representation of one cron
// firing after its schedule cursor claim has succeeded.
type OccurrenceSnapshot struct {
	ID         string
	ScheduleID string
	Execution  ExecutionSnapshot
	DueAt      time.Time
	FiredAt    time.Time
	NextRunAt  time.Time
	SessionID  string
	RunID      string
}

// Occurrence is one durable cron firing intent. It captures only the execution
// value plus the Schedule CAS/cursor facts needed to claim exactly once.
type Occurrence struct {
	id         occurrenceIdentity
	scheduleID resourceid.ScheduleID
	execution  Execution
	dueAt      time.Time
	firedAt    time.Time
	nextRunAt  time.Time
	sessionID  string
	runID      string
}

// Claim binds one immutable occurrence to the exact Schedule revision and due
// cursor it intends to consume. A claimed/pending Occurrence no longer carries
// this precondition, so zero never doubles as a lifecycle marker.
type Claim struct {
	occurrence       Occurrence
	expectedRevision exactint.Counter
}

// Acceptance identifies the exact durable occurrence and Run whose opening
// committed together. Private fields prevent argument transposition and
// partial ownership proofs at the persistence boundary.
type Acceptance struct {
	occurrenceID occurrenceIdentity
	runID        string
}

// RunRecord is the manual execution fact that becomes durable with the Run
// opening. The aggregate constructs it so identity and time cannot drift apart.
type RunRecord struct {
	scheduleID resourceid.ScheduleID
	ranAt      time.Time
}

// RunRequest is the immutable input for launching saved instructions. Manual
// requests carry stable Session/Run identities and one Run record but no cron
// occurrence; cron requests carry the occurrence and the same stable identities.
type RunRequest struct {
	scheduleID   resourceid.ScheduleID
	execution    Execution
	manualRecord *RunRecord
	occurrenceID occurrenceIdentity
	sessionID    string
	runID        string
}

// NewClaim captures one due cursor and derives the next cursor from the firing
// instant. Stable Run identities are supplied before persistence so a
// crash/retry cannot create a second Run.
func NewClaim(s Schedule, sessionID, runID string, firedAt time.Time) (Claim, error) {
	if err := s.Validate(); err != nil {
		return Claim{}, err
	}
	if !s.enabled || s.nextRunAt.IsZero() {
		return Claim{}, errors.New("schedule: only an enabled due schedule can form an occurrence")
	}
	firedAt = canonicalTime(firedAt)
	if firedAt.IsZero() {
		return Claim{}, errors.New("schedule: occurrence firing time is required")
	}
	nextRunAt, err := NextRun(s.cron, firedAt)
	if err != nil {
		return Claim{}, err
	}
	occurrenceID, err := newOccurrenceIdentity(s.id, s.nextRunAt)
	if err != nil {
		return Claim{}, err
	}
	occurrence := Occurrence{
		id: occurrenceID, scheduleID: s.id,
		execution: s.Execution(), dueAt: s.nextRunAt, firedAt: firedAt,
		nextRunAt: nextRunAt, sessionID: sessionID, runID: runID,
	}
	value := Claim{occurrence: occurrence, expectedRevision: s.revision}
	if err := value.Validate(); err != nil {
		return Claim{}, err
	}
	return value, nil
}

// Validate verifies the immutable durable firing value.
func (o Occurrence) Validate() error {
	if err := o.id.Validate(); err != nil {
		return err
	}
	if _, err := parseScheduleID(o.scheduleID.String()); err != nil {
		return fmt.Errorf("schedule: occurrence: %w", err)
	}
	if o.id.scheduleID != o.scheduleID {
		return errors.New("schedule: occurrence identity belongs to another Schedule")
	}
	if o.sessionID == "" || o.runID == "" {
		return errors.New("schedule: occurrence identities are required")
	}
	if _, err := resourceid.ParseSession(o.sessionID); err != nil {
		return fmt.Errorf("schedule: occurrence: %w", err)
	}
	if _, err := resourceid.ParseRun(o.runID); err != nil {
		return fmt.Errorf("schedule: occurrence: %w", err)
	}
	if err := o.execution.Validate(); err != nil {
		return err
	}
	if o.dueAt.IsZero() || o.firedAt.IsZero() || o.nextRunAt.IsZero() {
		return errors.New("schedule: occurrence times are required")
	}
	if o.id.dueMillis != o.dueAt.UnixMilli() {
		return errors.New("schedule: occurrence identity and due cursor disagree")
	}
	if o.firedAt.Before(o.dueAt) {
		return errors.New("schedule: occurrence fired before it was due")
	}
	if !o.nextRunAt.After(o.dueAt) || !o.nextRunAt.After(o.firedAt) {
		return errors.New("schedule: occurrence next cursor must follow its history")
	}
	return nil
}

// Validate verifies the claim's occurrence and exact non-zero CAS identity.
func (c Claim) Validate() error {
	if err := c.occurrence.Validate(); err != nil {
		return err
	}
	if c.expectedRevision.IsZero() {
		return ErrRevisionRequired
	}
	return nil
}

// RestoreOccurrence reconstructs a durable pending firing without exposing a
// mutable half-aggregate to callers that reconstruct stored state.
func RestoreOccurrence(snapshot OccurrenceSnapshot) (Occurrence, error) {
	occurrenceID, err := parseOccurrenceIdentity(snapshot.ID)
	if err != nil {
		return Occurrence{}, err
	}
	scheduleID, err := parseScheduleID(snapshot.ScheduleID)
	if err != nil {
		return Occurrence{}, err
	}
	execution, err := RestoreExecution(snapshot.Execution)
	if err != nil {
		return Occurrence{}, err
	}
	value := Occurrence{
		id: occurrenceID, scheduleID: scheduleID,
		execution: execution, dueAt: canonicalTime(snapshot.DueAt), firedAt: canonicalTime(snapshot.FiredAt),
		nextRunAt: canonicalTime(snapshot.NextRunAt), sessionID: snapshot.SessionID,
		runID: snapshot.RunID,
	}
	if err := value.Validate(); err != nil {
		return Occurrence{}, err
	}
	return value, nil
}

// Snapshot returns the complete durable occurrence representation.
func (o Occurrence) Snapshot() OccurrenceSnapshot {
	return OccurrenceSnapshot{
		ID: o.id.String(), ScheduleID: o.scheduleID.String(), Execution: o.execution.Snapshot(),
		DueAt: o.dueAt, FiredAt: o.firedAt, NextRunAt: o.nextRunAt,
		SessionID: o.sessionID, RunID: o.runID,
	}
}

func (o Occurrence) ID() string           { return o.id.String() }
func (o Occurrence) ScheduleID() string   { return o.scheduleID.String() }
func (o Occurrence) Execution() Execution { return o.execution }
func (o Occurrence) DueAt() time.Time     { return o.dueAt }
func (o Occurrence) FiredAt() time.Time   { return o.firedAt }
func (o Occurrence) NextRunAt() time.Time { return o.nextRunAt }
func (o Occurrence) SessionID() string    { return o.sessionID }
func (o Occurrence) RunID() string        { return o.runID }

// Occurrence returns the immutable firing produced if this claim commits.
func (c Claim) Occurrence() Occurrence { return c.occurrence }

// ExpectedRevision returns the exact Schedule revision this claim consumes.
func (c Claim) ExpectedRevision() uint64 { return c.expectedRevision.Value() }

// NewAcceptance creates an exact occurrence-to-Run ownership proof.
func NewAcceptance(occurrenceID, runID string) (Acceptance, error) {
	parsedOccurrenceID, err := parseOccurrenceIdentity(occurrenceID)
	if err != nil {
		return Acceptance{}, err
	}
	value := Acceptance{occurrenceID: parsedOccurrenceID, runID: runID}
	if err := value.Validate(); err != nil {
		return Acceptance{}, err
	}
	return value, nil
}

// Validate rejects partial occurrence ownership.
func (a Acceptance) Validate() error {
	if err := a.occurrenceID.Validate(); err != nil {
		return err
	}
	if a.runID == "" {
		return errors.New("schedule: acceptance identities are required")
	}
	if _, err := resourceid.ParseRun(a.runID); err != nil {
		return fmt.Errorf("schedule: acceptance: %w", err)
	}
	return nil
}

func (a Acceptance) OccurrenceID() string { return a.occurrenceID.String() }
func (a Acceptance) RunID() string        { return a.runID }

// RecordRun forms the manual execution fact owned by the Run opening without
// changing the cron cursor. The store advances it from the current revision.
func (s Schedule) RecordRun(ranAt time.Time) (RunRecord, error) {
	if err := s.Validate(); err != nil {
		return RunRecord{}, err
	}
	value := RunRecord{scheduleID: s.id, ranAt: canonicalTime(ranAt)}
	if err := value.Validate(); err != nil {
		return RunRecord{}, err
	}
	if value.ranAt.Before(s.createdAt) {
		return RunRecord{}, errors.New("schedule: recorded run precedes creation")
	}
	return value, nil
}

// Validate rejects an incomplete manual execution fact.
func (r RunRecord) Validate() error {
	if _, err := parseScheduleID(r.scheduleID.String()); err != nil {
		return fmt.Errorf("schedule: recorded run: %w", err)
	}
	if r.ranAt.IsZero() {
		return errors.New("schedule: recorded run time is required")
	}
	return nil
}

func (r RunRecord) ScheduleID() string { return r.scheduleID.String() }
func (r RunRecord) RanAt() time.Time   { return r.ranAt }

// ManualRunRequest captures an aggregate's current instructions and the run
// fact that must commit with its Run opening. It does not claim or advance the
// cron cursor.
func ManualRunRequest(s Schedule, sessionID, runID string, ranAt time.Time) (RunRequest, error) {
	record, err := s.RecordRun(ranAt)
	if err != nil {
		return RunRequest{}, err
	}
	value := RunRequest{
		scheduleID: s.id, execution: s.Execution(), manualRecord: &record,
		sessionID: sessionID, runID: runID,
	}
	return value, value.Validate()
}

// RunRequest returns the stable launch input owned by this durable occurrence.
func (o Occurrence) RunRequest() RunRequest {
	return RunRequest{
		scheduleID: o.scheduleID, execution: o.execution, occurrenceID: o.id,
		sessionID: o.sessionID, runID: o.runID,
	}
}

// Validate rejects partial durable identity. A launch is either manual with
// one aggregate-owned Run record or occurrence-backed with every stable
// identity present.
func (r RunRequest) Validate() error {
	if _, err := parseScheduleID(r.scheduleID.String()); err != nil {
		return fmt.Errorf("schedule: run request: %w", err)
	}
	if err := r.execution.Validate(); err != nil {
		return err
	}
	if r.manualRecord != nil {
		if r.occurrenceID.String() != "" {
			return errors.New("schedule: run request mixes manual and occurrence identity")
		}
		if err := r.manualRecord.Validate(); err != nil {
			return err
		}
		if r.manualRecord.scheduleID != r.scheduleID {
			return errors.New("schedule: manual run record belongs to another Schedule")
		}
		if r.sessionID == "" || r.runID == "" {
			return errors.New("schedule: manual run request identities are required")
		}
		if _, err := resourceid.ParseSession(r.sessionID); err != nil {
			return fmt.Errorf("schedule: run request: %w", err)
		}
		if _, err := resourceid.ParseRun(r.runID); err != nil {
			return fmt.Errorf("schedule: run request: %w", err)
		}
		return nil
	}
	present := 0
	for _, value := range [...]string{r.occurrenceID.String(), r.sessionID, r.runID} {
		if value != "" {
			present++
		}
	}
	if present == 0 {
		return errors.New("schedule: run request has no execution ownership")
	}
	if present != 3 {
		return errors.New("schedule: run request has partial durable identity")
	}
	if err := r.occurrenceID.Validate(); err != nil {
		return err
	}
	if r.occurrenceID.scheduleID != r.scheduleID {
		return errors.New("schedule: run request occurrence belongs to another Schedule")
	}
	if _, err := resourceid.ParseSession(r.sessionID); err != nil {
		return fmt.Errorf("schedule: run request: %w", err)
	}
	if _, err := resourceid.ParseRun(r.runID); err != nil {
		return fmt.Errorf("schedule: run request: %w", err)
	}
	return nil
}

func (r RunRequest) ScheduleID() string   { return r.scheduleID.String() }
func (r RunRequest) Execution() Execution { return r.execution }
func (r RunRequest) ManualRecord() (RunRecord, bool) {
	if r.manualRecord == nil {
		return RunRecord{}, false
	}
	return *r.manualRecord, true
}
func (r RunRequest) OccurrenceID() string { return r.occurrenceID.String() }
func (r RunRequest) SessionID() string    { return r.sessionID }
func (r RunRequest) RunID() string        { return r.runID }

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
	if err := s.ValidateStored(); err != nil {
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
	if s.instructions == "" {
		return ErrInstructionsRequired
	}
	if s.cron == "" {
		return ErrCronRequired
	}
	if err := ValidateCron(s.cron); err != nil {
		return err
	}
	return nil
}

// ValidateStored remains the persistence-facing spelling for aggregate
// validation; unlike the former public-field model, every Schedule is stored.
func (s Schedule) ValidateStored() error { return s.Validate() }

// NextRevision returns the only legal successor of this stored Schedule's
// current revision. Persistence paths that mutate operational fields use this
// behavior instead of spelling arithmetic in SQL.
func (s Schedule) NextRevision() (uint64, error) {
	if err := s.ValidateStored(); err != nil {
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

// Execution returns the immutable instructions a manual or cron firing runs.
func (s Schedule) Execution() Execution {
	return Execution{
		title: s.title, instructions: s.instructions, cwd: s.cwd,
		modelSelection: s.modelSelection, cron: s.cron,
	}
}

// RestoreExecution reconstructs a durable firing snapshot without pretending
// it is a complete Schedule aggregate.
func RestoreExecution(snapshot ExecutionSnapshot) (Execution, error) {
	value := Execution{
		title: snapshot.Title, instructions: snapshot.Instructions, cwd: snapshot.CWD,
		modelSelection: snapshot.ModelSelection, cron: snapshot.Cron,
	}
	if err := value.Validate(); err != nil {
		return Execution{}, err
	}
	return value, nil
}

func (e Execution) Validate() error {
	if err := e.modelSelection.Validate(); err != nil {
		return fmt.Errorf("schedule: execution model selection: %w", err)
	}
	if e.instructions == "" {
		return ErrInstructionsRequired
	}
	if e.cron == "" {
		return ErrCronRequired
	}
	return ValidateCron(e.cron)
}

func (e Execution) Snapshot() ExecutionSnapshot {
	return ExecutionSnapshot{
		Title: e.title, Instructions: e.instructions, CWD: e.cwd,
		ModelSelection: e.modelSelection, Cron: e.cron,
	}
}

func (e Execution) Title() string                      { return e.title }
func (e Execution) Instructions() string               { return e.instructions }
func (e Execution) CWD() string                        { return e.cwd }
func (e Execution) ModelSelection() modelref.Selection { return e.modelSelection }
func (e Execution) Cron() string                       { return e.cron }

const durableTimePrecision = time.Millisecond

func canonicalTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Truncate(durableTimePrecision)
}

// ValidateCron reports whether spec is a parseable 5-field cron expression
// (the boundary check create/update run before persisting).
func ValidateCron(spec string) error {
	if _, err := cron.ParseStandard(spec); err != nil {
		return fmt.Errorf("%w %q: %w", ErrInvalidCron, spec, err)
	}
	return nil
}

// NextRun returns the first time spec fires strictly after `after`. It is the
// single source of NextRunAt — create/update compute it from the new cron, and
// the worker advances it after each firing (so a schedule missed during
// downtime fires once on restart, then jumps to its next future slot rather
// than replaying every missed occurrence).
func NextRun(spec string, after time.Time) (time.Time, error) {
	sched, err := cron.ParseStandard(spec)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w %q: %w", ErrInvalidCron, spec, err)
	}
	return sched.Next(after), nil
}
