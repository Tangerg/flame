package goals

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/samber/lo"
)

// Outcome is a terminal result a Goal Run may report. Any other value is
// refused as ReportInvalidOutcome.
type Outcome string

const (
	OutcomeCompleted Outcome = "completed"
	OutcomeBlocked   Outcome = "blocked"
)

var outcomes = [...]Outcome{OutcomeCompleted, OutcomeBlocked}

func (o Outcome) Valid() bool { return slices.Contains(outcomes[:], o) }

func (Outcome) JSONSchemaEnum() []string {
	values := make([]string, len(outcomes))
	for index, outcome := range outcomes {
		values[index] = string(outcome)
	}
	return values
}

// ReportCommand is a model-originated terminal outcome for the active Goal.
// IncarnationID is required and identifies the Goal that admitted this Run.
// Reason explains a blocked outcome and must be empty for a completed one;
// surrounding whitespace is not part of it.
type ReportCommand struct {
	SessionID     string
	IncarnationID string
	Outcome       Outcome
	Reason        string
}

// ReportResult identifies the recoverable outcome of one report.
type ReportResult string

const (
	ReportApplied         ReportResult = "applied"
	ReportNoActiveGoal    ReportResult = "noActiveGoal"
	ReportSuperseded      ReportResult = "superseded"
	ReportConflict        ReportResult = "conflict"
	ReportReasonRequired  ReportResult = "reasonRequired"
	ReportReasonForbidden ReportResult = "reasonForbidden"
	ReportInvalidOutcome  ReportResult = "invalidOutcome"
)

// OutcomeReporter owns terminal outcome validation and compare-and-swap.
type OutcomeReporter struct {
	workflow Workflow
	goals    Store
	now      func() time.Time
}

// NewOutcomeReporter constructs terminal outcome reporting over the required store.
func NewOutcomeReporter(store Store, workflow Workflow) (*OutcomeReporter, error) {
	if lo.IsNil(store) || lo.IsNil(workflow) {
		return nil, errors.New("goals: outcome store is required")
	}
	return &OutcomeReporter{workflow: workflow, goals: store, now: time.Now}, nil
}

// Report applies one model-declared terminal outcome to the active Goal.
func (o *OutcomeReporter) Report(ctx context.Context, cmd ReportCommand) (ReportResult, error) {
	if _, err := goalref.ParseIncarnation(cmd.IncarnationID); err != nil {
		return "", fmt.Errorf("goals: report: %w", err)
	}
	reason := strings.TrimSpace(cmd.Reason)
	if !cmd.Outcome.Valid() {
		return ReportInvalidOutcome, nil
	}
	if cmd.Outcome == OutcomeCompleted && reason != "" {
		return ReportReasonForbidden, nil
	}
	g, ok, err := loadGoal(ctx, o.goals, cmd.SessionID)
	if err != nil {
		return "", err
	}
	if !ok || g.Status() != goal.StatusActive {
		return ReportNoActiveGoal, nil
	}
	if cmd.IncarnationID != g.IncarnationID() {
		return ReportSuperseded, nil
	}
	expected := g.Version()
	var replacement goal.Goal
	if cmd.Outcome == OutcomeCompleted {
		replacement, err = transition(ctx, o.workflow, g, goalworkflow.Request{Signal: goalworkflow.Complete}, g.IncarnationID(), o.now())
	} else {
		replacement, err = transition(ctx, o.workflow, g, goalworkflow.Request{Signal: goalworkflow.Block, Explanation: reason}, g.IncarnationID(), o.now())
		if errors.Is(err, goal.ErrBlockExplanationRequired) {
			return ReportReasonRequired, nil
		}
	}
	if err != nil {
		return "", err
	}
	change, err := goal.NewReplacement(expected, replacement)
	if err != nil {
		return "", err
	}
	applied, err := o.goals.Save(ctx, change)
	if err != nil {
		return "", err
	}
	if !applied {
		return ReportConflict, nil
	}
	return ReportApplied, nil
}
