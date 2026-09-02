package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"
)

func (a *app) ShowGoal() {
	if a.goals == nil {
		a.message("this runtime composition has no goal service")
		return
	}
	a.executeRuntimeReaderQuery(a.goalReaderQuery())
}

func (a *app) goalReaderQuery() runtimeReaderQuery {
	sessionID := a.session.current.ID
	return runtimeReaderQuery{
		status: "loading session goal",
		mode:   runtimeReaderGoal,
		read: func(ctx context.Context) (readerDocument, error) {
			current, exists, err := a.goals.GetGoal(ctx, sessionID)
			if err != nil {
				return readerDocument{}, err
			}
			return goalDocument(current, exists), nil
		},
	}
}

func goalDocument(current protocol.Goal, exists bool) readerDocument {
	if !exists {
		return paragraphDocument("Session goal", "none", []string{"No autonomous goal is active or paused for this session."})
	}
	lines := []string{
		"objective  " + current.Objective,
		"status     " + string(current.Status),
		fmt.Sprintf("used       %d runs · %d steps · %s", current.Used.Runs, current.Used.Steps, goalCostLabel(current.Used)),
	}
	model := "runtime default"
	if current.Provider != "" {
		model = current.Provider + "/" + current.Model
		if current.ReasoningEffort != "" {
			model += " · reasoning " + current.ReasoningEffort
		}
	}
	lines = append(lines, "model      "+model)
	budget := []string{}
	if current.Budget != nil {
		if current.Budget.MaxRuns != nil {
			budget = append(budget, fmt.Sprintf("%d runs", *current.Budget.MaxRuns))
		}
		if current.Budget.MaxSteps != nil {
			budget = append(budget, fmt.Sprintf("%d steps", *current.Budget.MaxSteps))
		}
		if current.Budget.MaxCostUSD != nil {
			budget = append(budget, fmt.Sprintf("$%.4f", *current.Budget.MaxCostUSD))
		}
	}
	if len(budget) == 0 {
		budget = append(budget, "unbounded")
	}
	lines = append(lines, "budget     "+strings.Join(budget, " · "))
	if current.Reason != nil {
		reason := string(current.Reason.Code)
		if current.Reason.Detail != "" {
			reason += " · " + current.Reason.Detail
		}
		lines = append(lines, "reason     "+reason)
	}
	return paragraphDocument("Session goal", string(current.Status), lines)
}

func goalCostLabel(used protocol.GoalUsage) string {
	if used.CostUSD != nil {
		return fmt.Sprintf("$%.4f", *used.CostUSD)
	}
	if used.Runs == 0 {
		return "no cost yet"
	}
	return "cost unavailable"
}

func (a *app) StartGoal(objective string) error {
	if a.goals == nil {
		return errors.New("this runtime composition has no goal service")
	}
	start := protocol.StartGoalRequest{
		SessionID: a.session.current.ID, Objective: objective,
		Provider: a.options.Provider, Model: a.options.Model, ReasoningEffort: a.options.ReasoningEffort,
	}
	return a.changeGoal("starting session goal", func(ctx context.Context) (protocol.Goal, error) {
		return a.goals.StartGoal(ctx, start)
	})
}

func (a *app) UpdateGoal(objective string) error {
	if a.goals == nil {
		return errors.New("this runtime composition has no goal service")
	}
	update := protocol.UpdateGoalRequest{SessionID: a.session.current.ID, Objective: objective}
	return a.changeGoal("updating session goal", func(ctx context.Context) (protocol.Goal, error) {
		return a.goals.UpdateGoal(ctx, update)
	})
}

func (a *app) ClearGoal() error {
	if a.goals == nil {
		return errors.New("this runtime composition has no goal service")
	}
	presentation := a.session.context
	sessionID := a.session.current.ID
	label := "clearing session goal"
	a.status.note(label)
	started := a.runAdmissionMutation(goalOperation, false,
		func(ctx context.Context) (struct{}, error) {
			return struct{}{}, a.goals.ClearGoal(ctx, sessionID)
		},
		func(_ struct{}, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			if a.session.context.current(presentation) {
				a.header.SetGoal(nil)
				a.setRuntimeReader(runtimeReaderGoal)
				a.dialogs.workspaceReader = workspaceReaderNone
				a.openReaderDocument(goalDocument(protocol.Goal{}, false))
			}
			a.status.note("goal · cleared")
		},
	)
	if !started {
		return errors.New("another goal operation is running")
	}
	return nil
}

func (a *app) StopGoal() error {
	if a.goals == nil {
		return errors.New("this runtime composition has no goal service")
	}
	sessionID := a.session.current.ID
	return a.changeGoal("stopping session goal", func(ctx context.Context) (protocol.Goal, error) {
		return a.goals.StopGoal(ctx, sessionID)
	})
}

func (a *app) ResumeGoal() error {
	if a.goals == nil {
		return errors.New("this runtime composition has no goal service")
	}
	sessionID := a.session.current.ID
	return a.changeGoal("resuming session goal", func(ctx context.Context) (protocol.Goal, error) {
		return a.goals.ResumeGoal(ctx, sessionID)
	})
}

func (a *app) changeGoal(label string, change func(context.Context) (protocol.Goal, error)) error {
	presentation := a.session.context
	a.status.note(label)
	started := a.runAdmissionMutation(goalOperation, false, change, func(current protocol.Goal, err error) {
		if err != nil {
			a.message(label + " failed: " + err.Error())
			return
		}
		if a.session.context.current(presentation) {
			a.header.SetGoal(&current)
			a.setRuntimeReader(runtimeReaderGoal)
			a.dialogs.workspaceReader = workspaceReaderNone
			a.openReaderDocument(goalDocument(current, true))
		}
		a.status.note("goal · " + string(current.Status))
	})
	if !started {
		return errors.New("another goal operation is running")
	}
	return nil
}
