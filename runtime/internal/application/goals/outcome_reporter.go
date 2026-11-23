package goals

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/samber/lo"
)

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

type ReportCommand struct {
	SessionID     identity.SessionID
	IncarnationID goalref.IncarnationID
	Outcome       Outcome
	Reason        string
}

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

type OutcomeReporter struct {
	workflow Workflow
	goals    Store
	now      func() time.Time
}

func NewOutcomeReporter(store Store, workflow Workflow) (*OutcomeReporter, error) {
	if lo.IsNil(store) || lo.IsNil(workflow) {
		return nil, errors.New("goals: outcome store is required")
	}
	return &OutcomeReporter{workflow: workflow, goals: store, now: time.Now}, nil
}

func (o *OutcomeReporter) Report(ctx context.Context, cmd ReportCommand) (ReportResult, error) {
	if err := cmd.IncarnationID.Validate(); err != nil {
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
