package plan

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/flame/packages/go-sdk/planworkflow"
)

type Status string

const (
	StatusPending    Status = Status(planworkflow.PlanStatusPending)
	StatusInProgress Status = Status(planworkflow.PlanStatusInProgress)
	StatusCompleted  Status = Status(planworkflow.PlanStatusCompleted)
)

var statuses = [...]Status{StatusPending, StatusInProgress, StatusCompleted}

func (s Status) Valid() bool {
	return slices.Contains(statuses[:], s)
}

func (Status) JSONSchemaEnum() []string {
	values := make([]string, len(statuses))
	for index, status := range statuses {
		values[index] = string(status)
	}
	return values
}

type Step struct {
	Description string
	Status      Status
}

var (
	ErrInvalid          = errors.New("plan: invalid plan")
	ErrRevisionConflict = errors.New("plan: revision conflict")
)

func ProposalOf(steps []Step) planworkflow.Proposal {
	proposal := planworkflow.Proposal{Steps: make([]planworkflow.Step, len(steps))}
	for index, step := range steps {
		proposal.Steps[index] = planworkflow.Step{Description: step.Description, Status: planworkflow.PlanStatus(step.Status)}
	}
	return proposal
}

func StepsOf(proposal planworkflow.Proposal) []Step {
	steps := make([]Step, len(proposal.Steps))
	for i, step := range proposal.Steps {
		steps[i] = Step{Description: step.Description, Status: Status(step.Status)}
	}
	return steps
}

func ValidateSteps(steps []Step) error {
	for index, step := range ProposalOf(steps).Steps {
		if err := step.Validate(); err != nil {
			return fmt.Errorf("%w: step %d: %w", ErrInvalid, index, err)
		}
	}
	return nil
}

type Snapshot struct {
	Source    extension.ContributionSource
	Steps     []Step
	Revision  uint64
	UpdatedAt time.Time
}

type State struct {
	source    extension.ContributionSource
	steps     []Step
	revision  exactint.Counter
	updatedAt time.Time
}

type Current struct{ state *State }

type Version struct {
	revision  exactint.Counter
	committed bool
}

func Restore(snapshot Snapshot) (State, error) {
	revision, err := exactint.Restore(snapshot.Revision)
	if err != nil {
		return State{}, fmt.Errorf("%w: revision: %w", ErrInvalid, err)
	}
	state := State{source: snapshot.Source,
		steps:     cloneSteps(snapshot.Steps),
		revision:  revision,
		updatedAt: snapshot.UpdatedAt.UTC(),
	}
	if err := state.Validate(); err != nil {
		return State{}, err
	}
	return state, nil
}

type Document struct {
	Steps  []Step
	Source extension.ContributionSource
}

func (s State) Document() Document { return Document{Source: s.source, Steps: s.Steps()} }

func (c Current) Reference(doc Document, now time.Time) (State, error) {
	if doc.Source != (extension.ContributionSource{}) {
		if err := doc.Source.Validate(); err != nil {
			return State{}, err
		}
	}
	next, err := c.Replace(doc.Steps, now)
	if err != nil {
		return State{}, err
	}
	next.source = doc.Source
	return next, nil
}

func CurrentOf(state State) (Current, error) {
	if err := state.Validate(); err != nil {
		return Current{}, err
	}
	return Current{state: &state}, nil
}

func (c Current) State() (State, bool) {
	if c.state == nil {
		return State{}, false
	}
	return *c.state, true
}

func (c Current) Steps() []Step {
	if c.state == nil {
		return nil
	}
	return c.state.Steps()
}

func (c Current) Version() Version {
	if c.state == nil {
		return Version{}
	}
	return Version{revision: c.state.revision, committed: true}
}

func (c Current) Replace(steps []Step, updatedAt time.Time) (State, error) {
	if c.state == nil {
		return create(steps, updatedAt)
	}
	return c.state.Replace(steps, updatedAt)
}

func (s State) Replace(steps []Step, updatedAt time.Time) (State, error) {
	if s.revision.IsZero() {
		return State{}, fmt.Errorf("%w: replaced Plan state was never written", ErrInvalid)
	}
	if err := ValidateSteps(steps); err != nil {
		return State{}, err
	}
	updatedAt = updatedAt.UTC()
	if updatedAt.IsZero() {
		return State{}, fmt.Errorf("%w: replacement time is required", ErrInvalid)
	}
	if err := instant.Validate(updatedAt); err != nil {
		return State{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if !s.updatedAt.IsZero() && updatedAt.Before(s.updatedAt) {
		return State{}, fmt.Errorf("%w: replacement time precedes current state", ErrInvalid)
	}
	revision, err := s.revision.Next()
	if err != nil {
		return State{}, fmt.Errorf("%w: revision: %w", ErrInvalid, err)
	}
	return State{
		steps:     cloneSteps(steps),
		revision:  revision,
		updatedAt: updatedAt,
	}, nil
}

func create(steps []Step, updatedAt time.Time) (State, error) {
	state := State{steps: cloneSteps(steps), revision: exactint.First(), updatedAt: updatedAt.UTC()}
	if err := state.Validate(); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s State) Validate() error {
	if s.source != (extension.ContributionSource{}) {
		if err := s.source.Validate(); err != nil {
			return fmt.Errorf("%w: author: %w", ErrInvalid, err)
		}
	}

	if err := ValidateSteps(s.steps); err != nil {
		return err
	}
	if s.revision.IsZero() {
		return fmt.Errorf("%w: committed Plan revision must be positive", ErrInvalid)
	}
	if s.updatedAt.IsZero() {
		return fmt.Errorf("%w: committed Plan has no update time", ErrInvalid)
	}
	if err := instant.Validate(s.updatedAt); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return nil
}

func (s State) Steps() []Step { return cloneSteps(s.steps) }

func (s State) Revision() uint64 { return s.revision.Value() }

func (s State) UpdatedAt() time.Time { return s.updatedAt }

func (s State) Snapshot() Snapshot {
	return Snapshot{Source: s.source, Steps: cloneSteps(s.steps), Revision: s.revision.Value(), UpdatedAt: s.updatedAt}
}

func (v Version) IsUnwritten() bool { return !v.committed }

func (v Version) Revision() (uint64, bool) { return v.revision.Value(), v.committed }

func (v Version) AdvancesTo(next State) error {
	expected := exactint.First()
	if v.committed {
		var err error
		expected, err = v.revision.Next()
		if err != nil {
			return fmt.Errorf("%w: revision: %w", ErrInvalid, err)
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
		return []Step{}
	}
	return append([]Step(nil), steps...)
}
