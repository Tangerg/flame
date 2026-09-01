// Package goal owns the durable autonomous-objective aggregate. A Goal spans
// every Run launched for one objective incarnation; the owning use case drives the
// loop, while this package owns lifecycle, accounting, time, and version rules.
package goal

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// Status is one Goal's durable lifecycle state.
//
// StatusComplete is a transient committed state: the owning drive observes it,
// publishes completion, settles the terminal Run, and conditionally clears it.
type Status string

const (
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusBlocked  Status = "blocked"
	StatusComplete Status = "complete"
	firstRevision  int64  = 1
)

func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusPaused, StatusBlocked, StatusComplete:
		return true
	default:
		return false
	}
}

// Snapshot is the technical reconstruction boundary, not a mutation surface.
type Snapshot struct {
	SessionID      string
	Objective      string
	Status         Status
	ReasonCode     ReasonCode
	ReasonDetail   string
	ModelSelection modelref.Selection
	Capabilities   run.Capabilities
	Budget         Budget
	Used           Usage
	IncarnationID  string
	Revision       int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Goal is one committed autonomous objective. Private fields make every next
// committed value pass through a domain transition.
type Goal struct {
	sessionID     string
	objective     string
	status        Status
	reason        Reason
	selection     modelref.Selection
	capabilities  run.Capabilities
	budget        Budget
	used          Usage
	incarnationID goalref.IncarnationID
	revision      int64
	createdAt     time.Time
	updatedAt     time.Time
}

// Current is the explicit optional Goal value for one Session. Its zero value
// is invalid rather than ambiguously meaning "not found"; use [Unwritten].
type Current struct {
	sessionID string
	goal      *Goal
}

// Version is the typed CAS identity of a Current Goal. Presence is explicit;
// revision zero and an empty incarnation are not absence sentinels.
type Version struct {
	sessionID     string
	incarnationID goalref.IncarnationID
	revision      int64
	committed     bool
}

var (
	errSessionRequired     = errors.New("goal: session ID is required")
	errObjectiveRequired   = errors.New("goal: objective is required")
	ErrInvalid             = errors.New("goal: invalid Goal")
	ErrInvalidTransition   = errors.New("goal: invalid lifecycle transition")
	ErrBudgetExhausted     = errors.New("goal: budget exhausted")
	ErrNotResumable        = errors.New("goal: status is not resumable")
	ErrNotEditable         = errors.New("goal: status is not editable")
	ErrRunIdentityConflict = errors.New("goal: Run identity conflict")
)

// Unwritten constructs the explicit absent Goal value for sessionID.
func Unwritten(sessionID string) (Current, error) {
	if err := validateSessionIdentity(sessionID); err != nil {
		return Current{}, err
	}
	return Current{sessionID: sessionID}, nil
}

// CurrentOf owns one validated committed Goal as its Session's latest value.
func CurrentOf(value Goal) (Current, error) {
	if err := value.ValidateSnapshot(); err != nil {
		return Current{}, err
	}
	owned := value.Clone()
	return Current{sessionID: owned.sessionID, goal: &owned}, nil
}

func (c Current) Validate() error {
	if err := validateSessionIdentity(c.sessionID); err != nil {
		return err
	}
	if c.goal == nil {
		return nil
	}
	if c.goal.sessionID != c.sessionID {
		return fmt.Errorf("%w: Current Session identity does not match Goal", ErrInvalid)
	}
	return c.goal.ValidateSnapshot()
}

func (c Current) Goal() (Goal, bool) {
	if c.goal == nil {
		return Goal{}, false
	}
	return c.goal.Clone(), true
}

func (c Current) SessionID() string { return c.sessionID }

func (c Current) Version() Version {
	if c.goal == nil {
		return Version{sessionID: c.sessionID}
	}
	return c.goal.Version()
}

// New constructs revision one of a fresh objective incarnation.
func New(
	sessionID, objective string,
	selection modelref.Selection,
	budget Budget,
	capabilities run.Capabilities,
	incarnationID string,
	now time.Time,
) (Goal, error) {
	return Restore(Snapshot{
		SessionID:      sessionID,
		Objective:      objective,
		Status:         StatusActive,
		ModelSelection: selection,
		Capabilities:   capabilities.Normalized(),
		Budget:         budget,
		IncarnationID:  incarnationID,
		Revision:       firstRevision,
		CreatedAt:      canonicalTime(now),
		UpdatedAt:      canonicalTime(now),
	})
}

// Restore reconstructs one committed Goal from a technical snapshot.
func Restore(snapshot Snapshot) (Goal, error) {
	incarnationID, err := goalref.ParseIncarnation(snapshot.IncarnationID)
	if err != nil {
		return Goal{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	reason := Reason{}
	if snapshot.ReasonCode != ReasonNone || snapshot.ReasonDetail != "" {
		var err error
		reason, err = newReason(snapshot.Status, snapshot.ReasonCode, snapshot.ReasonDetail)
		if err != nil {
			return Goal{}, err
		}
	}
	value := Goal{
		sessionID:     snapshot.SessionID,
		objective:     snapshot.Objective,
		status:        snapshot.Status,
		reason:        reason,
		selection:     snapshot.ModelSelection,
		capabilities:  snapshot.Capabilities.Clone(),
		budget:        snapshot.Budget,
		used:          snapshot.Used,
		incarnationID: incarnationID,
		revision:      snapshot.Revision,
		createdAt:     canonicalTime(snapshot.CreatedAt),
		updatedAt:     canonicalTime(snapshot.UpdatedAt),
	}
	if err := value.ValidateSnapshot(); err != nil {
		return Goal{}, err
	}
	return value, nil
}

func (g Goal) ValidateSnapshot() error {
	if err := validateSessionIdentity(g.sessionID); err != nil {
		return err
	}
	if strings.TrimSpace(g.objective) == "" {
		return errObjectiveRequired
	}
	if g.objective != strings.TrimSpace(g.objective) {
		return fmt.Errorf("%w: objective has surrounding whitespace", ErrInvalid)
	}
	if err := g.incarnationID.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if g.revision <= 0 {
		return fmt.Errorf("%w: revision must be positive", ErrInvalid)
	}
	if !g.status.Valid() {
		return fmt.Errorf("%w: unknown status %q", ErrInvalid, g.status)
	}
	if err := g.selection.Validate(); err != nil {
		return fmt.Errorf("%w: model selection: %v", ErrInvalid, err)
	}
	if !g.selection.Configured() {
		return fmt.Errorf("%w: exact model selection is required", ErrInvalid)
	}
	if err := g.capabilities.Validate(); err != nil {
		return fmt.Errorf("%w: capabilities: %v", ErrInvalid, err)
	}
	if err := g.budget.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := g.used.validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if g.createdAt.IsZero() || g.updatedAt.IsZero() {
		return fmt.Errorf("%w: creation and update times are required", ErrInvalid)
	}
	if g.createdAt.Location() != time.UTC || g.updatedAt.Location() != time.UTC {
		return fmt.Errorf("%w: times must be UTC", ErrInvalid)
	}
	if g.updatedAt.Before(g.createdAt) {
		return fmt.Errorf("%w: update time precedes creation", ErrInvalid)
	}
	switch g.status {
	case StatusActive, StatusComplete:
		if !g.reason.IsNone() || g.reason.detail != "" {
			return fmt.Errorf("%w: %s Goal must not carry a stop reason", ErrInvalid, g.status)
		}
	case StatusPaused, StatusBlocked:
		validated, err := newReason(g.status, g.reason.code, g.reason.detail)
		if err != nil || validated != g.reason {
			return fmt.Errorf("%w: invalid %s reason: %v", ErrInvalid, g.status, err)
		}
	}
	if g.status == StatusActive {
		if limit, exhausted := g.budget.exceeded(g.used); exhausted {
			return fmt.Errorf("%w: active Goal has exhausted %s budget", ErrInvalid, limit)
		}
	}
	return nil
}

func (g Goal) Snapshot() Snapshot {
	return Snapshot{
		SessionID: g.sessionID, Objective: g.objective, Status: g.status,
		ReasonCode: g.reason.code, ReasonDetail: g.reason.detail,
		ModelSelection: g.selection, Capabilities: g.capabilities.Clone(),
		Budget: g.budget, Used: g.used, IncarnationID: g.incarnationID.String(),
		Revision: g.revision, CreatedAt: g.createdAt, UpdatedAt: g.updatedAt,
	}
}

func (g Goal) Clone() Goal {
	g.capabilities = g.capabilities.Clone()
	return g
}

func (g Goal) SessionID() string                  { return g.sessionID }
func (g Goal) Objective() string                  { return g.objective }
func (g Goal) Status() Status                     { return g.status }
func (g Goal) Reason() Reason                     { return g.reason }
func (g Goal) ModelSelection() modelref.Selection { return g.selection }
func (g Goal) Capabilities() run.Capabilities     { return g.capabilities.Clone() }
func (g Goal) Budget() Budget                     { return g.budget }
func (g Goal) Used() Usage                        { return g.used }
func (g Goal) IncarnationID() string              { return g.incarnationID.String() }
func (g Goal) Revision() int64                    { return g.revision }
func (g Goal) CreatedAt() time.Time               { return g.createdAt }
func (g Goal) UpdatedAt() time.Time               { return g.updatedAt }

func (g Goal) Version() Version {
	return Version{sessionID: g.sessionID, incarnationID: g.incarnationID, revision: g.revision, committed: true}
}

func (v Version) IsUnwritten() bool             { return !v.committed }
func (v Version) SessionID() string             { return v.sessionID }
func (v Version) IncarnationID() (string, bool) { return v.incarnationID.String(), v.committed }
func (v Version) Revision() (int64, bool)       { return v.revision, v.committed }

func (v Version) Validate() error {
	if err := validateSessionIdentity(v.sessionID); err != nil {
		return err
	}
	if !v.committed {
		if v.incarnationID.String() != "" || v.revision != 0 {
			return fmt.Errorf("%w: unwritten Version carries committed identity", ErrInvalid)
		}
		return nil
	}
	if err := v.incarnationID.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if v.revision <= 0 {
		return fmt.Errorf("%w: committed Version revision must be positive", ErrInvalid)
	}
	return nil
}

// AdvancesTo accepts revision one for a fresh incarnation and exactly one
// revision of advancement inside an existing incarnation.
func (v Version) AdvancesTo(next Goal) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if err := next.ValidateSnapshot(); err != nil {
		return err
	}
	if next.sessionID != v.sessionID {
		return fmt.Errorf("%w: replacement belongs to another Session", ErrInvalid)
	}
	if !v.committed || next.incarnationID != v.incarnationID {
		if next.revision != firstRevision {
			return fmt.Errorf("%w: fresh incarnation must start at revision %d", ErrInvalid, firstRevision)
		}
		return nil
	}
	if v.revision == math.MaxInt64 {
		return fmt.Errorf("%w: revision exhausted", ErrInvalid)
	}
	if next.revision != v.revision+1 {
		return fmt.Errorf("%w: replacement revision %d does not follow %s", ErrInvalid, next.revision, v)
	}
	return nil
}

func (v Version) String() string {
	if !v.committed {
		return fmt.Sprintf("%s/unwritten", v.sessionID)
	}
	return fmt.Sprintf("%s/%s@%d", v.sessionID, v.incarnationID.String(), v.revision)
}

func (g Goal) Complete(now time.Time) (Goal, error) {
	if g.status != StatusActive {
		return Goal{}, fmt.Errorf("%w: cannot complete %s Goal", ErrInvalidTransition, g.status)
	}
	next, err := g.next(now)
	if err != nil {
		return Goal{}, err
	}
	next.status, next.reason = StatusComplete, Reason{}
	return next, next.ValidateSnapshot()
}

func (g Goal) Pause(code ReasonCode, detail string, now time.Time) (Goal, error) {
	if g.status != StatusActive {
		return Goal{}, fmt.Errorf("%w: cannot pause %s Goal", ErrInvalidTransition, g.status)
	}
	reason, err := newReason(StatusPaused, code, detail)
	if err != nil {
		return Goal{}, err
	}
	next, err := g.next(now)
	if err != nil {
		return Goal{}, err
	}
	next.status, next.reason = StatusPaused, reason
	return next, next.ValidateSnapshot()
}

// Stop returns the user-authored paused state after an owned drive has been
// quiesced. The terminal Run may already have derived a pause, block, or
// transient completion while quiescing; the explicit user command is the final
// lifecycle decision and replaces that reason without losing its accounting.
func (g Goal) Stop(now time.Time) (Goal, error) {
	next, err := g.next(now)
	if err != nil {
		return Goal{}, err
	}
	next.status = StatusPaused
	next.reason, err = newReason(StatusPaused, ReasonStoppedByUser, "")
	if err != nil {
		return Goal{}, err
	}
	return next, next.ValidateSnapshot()
}

func (g Goal) Block(code ReasonCode, detail string, now time.Time) (Goal, error) {
	if g.status != StatusActive {
		return Goal{}, fmt.Errorf("%w: cannot block %s Goal", ErrInvalidTransition, g.status)
	}
	reason, err := newReason(StatusBlocked, code, detail)
	if err != nil {
		return Goal{}, err
	}
	next, err := g.next(now)
	if err != nil {
		return Goal{}, err
	}
	next.status, next.reason = StatusBlocked, reason
	return next, next.ValidateSnapshot()
}

func (g Goal) Resume(now time.Time) (Goal, error) {
	if g.status != StatusPaused && g.status != StatusBlocked {
		return Goal{}, ErrNotResumable
	}
	if _, exhausted := g.budget.exceeded(g.used); exhausted {
		return Goal{}, ErrBudgetExhausted
	}
	next, err := g.next(now)
	if err != nil {
		return Goal{}, err
	}
	next.status, next.reason = StatusActive, Reason{}
	return next, next.ValidateSnapshot()
}

func (g Goal) ReviseObjective(objective, incarnationID string, now time.Time) (Goal, error) {
	return g.reviseObjective(objective, incarnationID, false, now)
}

// ReviseObjectiveAndResume keeps edit+reactivation to one durable replacement.
func (g Goal) ReviseObjectiveAndResume(objective, incarnationID string, now time.Time) (Goal, error) {
	if g.status != StatusPaused {
		return Goal{}, fmt.Errorf("%w: only a paused Goal can revise and resume", ErrInvalidTransition)
	}
	if _, exhausted := g.budget.exceeded(g.used); exhausted {
		return Goal{}, ErrBudgetExhausted
	}
	return g.reviseObjective(objective, incarnationID, true, now)
}

func (g Goal) reviseObjective(objective, incarnationID string, resume bool, now time.Time) (Goal, error) {
	if g.status == StatusComplete {
		return Goal{}, ErrNotEditable
	}
	if strings.TrimSpace(objective) == "" {
		return Goal{}, errObjectiveRequired
	}
	if objective != strings.TrimSpace(objective) {
		return Goal{}, fmt.Errorf("%w: objective has surrounding whitespace", ErrInvalid)
	}
	parsedIncarnationID, err := goalref.ParseIncarnation(incarnationID)
	if err != nil {
		return Goal{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if parsedIncarnationID == g.incarnationID {
		return Goal{}, fmt.Errorf("%w: revised objective requires a fresh incarnation", ErrInvalid)
	}
	updatedAt, err := g.transitionTime(now)
	if err != nil {
		return Goal{}, err
	}
	next := g.Clone()
	next.objective, next.incarnationID = objective, parsedIncarnationID
	next.revision, next.updatedAt = firstRevision, updatedAt
	if resume {
		next.status, next.reason = StatusActive, Reason{}
	}
	return next, next.ValidateSnapshot()
}

func (g Goal) next(now time.Time) (Goal, error) {
	if err := g.ValidateSnapshot(); err != nil {
		return Goal{}, err
	}
	if g.revision == math.MaxInt64 {
		return Goal{}, fmt.Errorf("%w: revision exhausted", ErrInvalid)
	}
	updatedAt, err := g.transitionTime(now)
	if err != nil {
		return Goal{}, err
	}
	next := g.Clone()
	next.revision++
	next.updatedAt = updatedAt
	return next, nil
}

func (g Goal) transitionTime(now time.Time) (time.Time, error) {
	now = canonicalTime(now)
	if now.IsZero() {
		return time.Time{}, fmt.Errorf("%w: transition time is required", ErrInvalid)
	}
	if now.Before(g.updatedAt) {
		return time.Time{}, fmt.Errorf("%w: transition time precedes current state", ErrInvalid)
	}
	return now, nil
}

func validateSessionIdentity(value string) error {
	if value == "" {
		return errSessionRequired
	}
	if _, err := resourceid.ParseSession(value); err != nil {
		return fmt.Errorf("%w: session ID: %v", ErrInvalid, err)
	}
	return nil
}

func canonicalTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC()
}
