// Package plan owns the session-scoped execution Plan maintained by the root
// Agent. A Plan is one ordered list of Steps; it is neither a second task
// system nor Plan-mode state. The aggregate owns replacement, validation, and
// monotonic revision semantics. Persistence and presentation remain outside.
package plan

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/internal/exactint"
)

// Status is one Step's execution state.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
)

// Valid reports whether s is a recognized Step status.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusInProgress, StatusCompleted:
		return true
	default:
		return false
	}
}

// Step is one ordered unit of work in a Plan.
type Step struct {
	Description string
	Status      Status
}

var (
	// ErrInvalid marks Plan state that violates a domain invariant.
	ErrInvalid = errors.New("plan: invalid plan")
	// ErrRevisionConflict marks a replacement based on a stale Plan revision.
	ErrRevisionConflict = errors.New("plan: revision conflict")
)

// ValidateSteps verifies an ordered Plan value. An empty value is valid and
// means clear the current Plan. At most one Step may be in progress so the
// execution focus remains unambiguous.
func ValidateSteps(steps []Step) error {
	inProgress := 0
	for index, step := range steps {
		if strings.TrimSpace(step.Description) == "" {
			return fmt.Errorf("%w: step %d description is required", ErrInvalid, index)
		}
		if !step.Status.Valid() {
			return fmt.Errorf("%w: step %d has unknown status %q", ErrInvalid, index, step.Status)
		}
		if step.Status == StatusInProgress {
			inProgress++
		}
	}
	if inProgress > 1 {
		return fmt.Errorf("%w: at most one step may be in_progress", ErrInvalid)
	}
	return nil
}

// Snapshot is the persistence representation of a Plan aggregate. It is a
// technical reconstruction boundary, not a mutation API.
type Snapshot struct {
	Steps     []Step
	Revision  uint64
	UpdatedAt time.Time
}

// State is one immutable committed Plan replacement. Its private representation
// may be shared by value; mutable step input and output are copied at the boundary.
// Whether a Session has ever committed a Plan belongs to [Current].
type State struct {
	steps     []Step
	revision  exactint.Counter
	updatedAt time.Time
}

// Current is the optional latest Plan aggregate for one Session. Its zero value
// explicitly means unwritten; a committed empty State is distinct and remains
// available through State.
type Current struct{ state *State }

// Version is the optimistic-concurrency identity of a Current Plan. Its zero
// value is the explicit unwritten version; committed revisions remain private
// so callers cannot pair a numeric sentinel with unrelated Plan content.
type Version struct {
	revision  exactint.Counter
	committed bool
}

// Restore reconstructs a Plan aggregate from a trusted persistence boundary
// while rechecking every invariant.
func Restore(snapshot Snapshot) (State, error) {
	revision, err := exactint.Restore(snapshot.Revision)
	if err != nil {
		return State{}, fmt.Errorf("%w: revision: %v", ErrInvalid, err)
	}
	state := State{
		steps:     cloneSteps(snapshot.Steps),
		revision:  revision,
		updatedAt: canonicalTime(snapshot.UpdatedAt),
	}
	if err := state.Validate(); err != nil {
		return State{}, err
	}
	return state, nil
}

// CurrentOf wraps one validated committed State as the latest Session value.
func CurrentOf(state State) (Current, error) {
	if err := state.Validate(); err != nil {
		return Current{}, err
	}
	return Current{state: &state}, nil
}

// Validate verifies the optional aggregate and its committed State.
func (c Current) Validate() error {
	if c.state == nil {
		return nil
	}
	return c.state.Validate()
}

// State returns the immutable committed State and whether one has been written.
func (c Current) State() (State, bool) {
	if c.state == nil {
		return State{}, false
	}
	return *c.state, true
}

// Steps returns the latest ordered value. Unwritten and explicitly cleared
// Plans both render as an empty list; callers that need to distinguish them use
// State or Version.
func (c Current) Steps() []Step {
	if c.state == nil {
		return nil
	}
	return c.state.Steps()
}

// Version returns the concurrency identity coupled to this optional value.
func (c Current) Version() Version {
	if c.state == nil {
		return Version{}
	}
	return Version{revision: c.state.revision, committed: true}
}

// Replace decides one committed whole-list replacement. An unwritten Current
// receives the first revision; an existing State advances once.
func (c Current) Replace(steps []Step, updatedAt time.Time) (State, error) {
	if err := c.Validate(); err != nil {
		return State{}, fmt.Errorf("%w: current value: %v", ErrInvalid, err)
	}
	if c.state == nil {
		return create(steps, updatedAt)
	}
	return c.state.Replace(steps, updatedAt)
}

// Replace returns the next complete Plan state. The caller supplies the clock
// value; the aggregate owns revision advancement and rejects time travel or
// revision overflow.
func (s State) Replace(steps []Step, updatedAt time.Time) (State, error) {
	if err := s.Validate(); err != nil {
		return State{}, fmt.Errorf("%w: current state: %v", ErrInvalid, err)
	}
	if err := ValidateSteps(steps); err != nil {
		return State{}, err
	}
	updatedAt = canonicalTime(updatedAt)
	if updatedAt.IsZero() {
		return State{}, fmt.Errorf("%w: replacement time is required", ErrInvalid)
	}
	if !s.updatedAt.IsZero() && updatedAt.Before(s.updatedAt) {
		return State{}, fmt.Errorf("%w: replacement time precedes current state", ErrInvalid)
	}
	revision, err := s.revision.Next()
	if err != nil {
		return State{}, fmt.Errorf("%w: revision: %v", ErrInvalid, err)
	}
	return State{
		steps:     cloneSteps(steps),
		revision:  revision,
		updatedAt: updatedAt,
	}, nil
}

func create(steps []Step, updatedAt time.Time) (State, error) {
	if err := ValidateSteps(steps); err != nil {
		return State{}, err
	}
	updatedAt = canonicalTime(updatedAt)
	if updatedAt.IsZero() {
		return State{}, fmt.Errorf("%w: replacement time is required", ErrInvalid)
	}
	return State{steps: cloneSteps(steps), revision: exactint.First(), updatedAt: updatedAt}, nil
}

// Validate verifies the aggregate's reconstruction and lifecycle invariants.
func (s State) Validate() error {
	if err := ValidateSteps(s.steps); err != nil {
		return err
	}
	if s.revision.IsZero() {
		return fmt.Errorf("%w: committed Plan revision must be positive", ErrInvalid)
	}
	if s.updatedAt.IsZero() {
		return fmt.Errorf("%w: committed Plan has no update time", ErrInvalid)
	}
	return nil
}

// Steps returns a defensive copy of the ordered Plan value.
func (s State) Steps() []Step { return cloneSteps(s.steps) }

// Revision returns the monotonic replacement revision.
func (s State) Revision() uint64 { return s.revision.Value() }

// UpdatedAt returns when this replacement was committed.
func (s State) UpdatedAt() time.Time { return s.updatedAt }

// Snapshot returns a defensive persistence representation.
func (s State) Snapshot() Snapshot {
	return Snapshot{Steps: cloneSteps(s.steps), Revision: s.revision.Value(), UpdatedAt: s.updatedAt}
}

// IsUnwritten reports whether v identifies the absence of a committed Plan.
func (v Version) IsUnwritten() bool { return !v.committed }

// Revision returns the committed revision and whether one exists.
func (v Version) Revision() (uint64, bool) { return v.revision.Value(), v.committed }

// Validate verifies that presence and numeric revision cannot contradict.
func (v Version) Validate() error {
	if v.committed && v.revision.IsZero() {
		return fmt.Errorf("%w: committed version must be positive", ErrInvalid)
	}
	if !v.committed && !v.revision.IsZero() {
		return fmt.Errorf("%w: unwritten version carries a revision", ErrInvalid)
	}
	return nil
}

// AdvancesTo verifies that next is exactly one replacement after v.
func (v Version) AdvancesTo(next State) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	expected := exactint.First()
	if v.committed {
		var err error
		expected, err = v.revision.Next()
		if err != nil {
			return fmt.Errorf("%w: revision: %v", ErrInvalid, err)
		}
	}
	if next.revision != expected {
		return fmt.Errorf("%w: replacement revision %d does not follow version %s", ErrInvalid, next.revision.Value(), v)
	}
	return nil
}

func (v Version) String() string {
	if !v.committed {
		return "unwritten"
	}
	return fmt.Sprintf("revision %d", v.revision.Value())
}

func cloneSteps(steps []Step) []Step {
	if len(steps) == 0 {
		return nil
	}
	return append([]Step(nil), steps...)
}

func canonicalTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC()
}
