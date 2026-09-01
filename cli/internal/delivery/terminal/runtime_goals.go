package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
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

func goalDocument(current agent.Goal, exists bool) readerDocument {
	if !exists {
		return paragraphDocument("Session goal", "none", []string{"No autonomous goal is active or paused for this session."})
	}
	lines := []string{
		"objective  " + current.Objective(),
		"status     " + string(current.Status()),
		fmt.Sprintf("used       %d runs · %d steps · $%.4f", current.Used().Runs(), current.Used().Steps(), current.Used().CostUSD()),
	}
	model := "runtime default"
	if current.Provider() != "" {
		model = current.Provider() + "/" + current.Model()
		if current.ReasoningEffort() != "" {
			model += " · reasoning " + current.ReasoningEffort()
		}
	}
	lines = append(lines, "model      "+model)
	budget := []string{}
	if value, limited := current.Budget().MaxRuns(); limited {
		budget = append(budget, fmt.Sprintf("%d runs", value))
	}
	if value, limited := current.Budget().MaxSteps(); limited {
		budget = append(budget, fmt.Sprintf("%d steps", value))
	}
	if value, limited := current.Budget().MaxCostUSD(); limited {
		budget = append(budget, fmt.Sprintf("$%.4f", value))
	}
	if len(budget) == 0 {
		budget = append(budget, "unbounded")
	}
	lines = append(lines, "budget     "+strings.Join(budget, " · "))
	if currentReason, present := current.Reason(); present {
		reason := string(currentReason.Code())
		if currentReason.Detail() != "" {
			reason += " · " + currentReason.Detail()
		}
		lines = append(lines, "reason     "+reason)
	}
	return paragraphDocument("Session goal", string(current.Status()), lines)
}

func (a *app) StartGoal(objective string) error {
	if a.goals == nil {
		return errors.New("this runtime composition has no goal service")
	}
	start := agent.StartGoal{
		SessionID: a.session.current.ID, Objective: strings.TrimSpace(objective),
		Provider: a.options.Provider, Model: a.options.Model, ReasoningEffort: a.options.ReasoningEffort,
		Budget: agent.UnlimitedGoalBudget(),
	}
	if err := start.Validate(); err != nil {
		return err
	}
	return a.changeGoal("starting session goal", func(ctx context.Context) (agent.Goal, error) {
		return a.goals.StartGoal(ctx, start)
	})
}

func (a *app) UpdateGoal(objective string) error {
	if a.goals == nil {
		return errors.New("this runtime composition has no goal service")
	}
	update := agent.UpdateGoal{SessionID: a.session.current.ID, Objective: strings.TrimSpace(objective)}
	if err := update.Validate(); err != nil {
		return err
	}
	return a.changeGoal("updating session goal", func(ctx context.Context) (agent.Goal, error) {
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
				a.openReaderDocument(goalDocument(agent.Goal{}, false))
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
	return a.changeGoal("stopping session goal", func(ctx context.Context) (agent.Goal, error) {
		return a.goals.StopGoal(ctx, sessionID)
	})
}

func (a *app) ResumeGoal() error {
	if a.goals == nil {
		return errors.New("this runtime composition has no goal service")
	}
	sessionID := a.session.current.ID
	return a.changeGoal("resuming session goal", func(ctx context.Context) (agent.Goal, error) {
		return a.goals.ResumeGoal(ctx, sessionID)
	})
}

func (a *app) changeGoal(label string, change func(context.Context) (agent.Goal, error)) error {
	presentation := a.session.context
	a.status.note(label)
	sessionID := a.session.current.ID
	work := func(ctx context.Context) (agent.Goal, error) {
		current, exists, err := a.goals.GetGoal(ctx, sessionID)
		if err != nil {
			return agent.Goal{}, err
		}
		if exists && !current.Status().AllowsLifecycleCommands() {
			return agent.Goal{}, errors.New("goal is completing final accounting; wait for the next runtime change")
		}
		return change(ctx)
	}
	started := a.runAdmissionMutation(goalOperation, false, work, func(current agent.Goal, err error) {
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
		a.status.note("goal · " + string(current.Status()))
	})
	if !started {
		return errors.New("another goal operation is running")
	}
	return nil
}
