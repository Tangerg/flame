package terminal

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/apps/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/apps/cli/internal/delivery/format"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type goalScreens struct {
	screenHost
	service Goals
	options *authoring.RunOptions
	header  *sessionHeader
}

func (g *goalScreens) ShowGoal() {
	g.reader.show(g.goalReaderQuery())
}

func (g *goalScreens) goalReaderQuery() *readerQuery {
	sessionID := g.session.current.ID
	return &readerQuery{
		status:  "loading session goal",
		follows: func(event changefeed.Event) bool { return goalInvalidationAffectsSession(event, sessionID) },
		read: func(ctx context.Context) (readerDocument, error) {
			current, exists, err := g.service.GetGoal(ctx, sessionID)
			if err != nil {
				return readerDocument{}, err
			}
			return goalDocument(current, exists), nil
		},
	}
}

func goalDocument(current protocol.Goal, exists bool) readerDocument {
	if !exists {
		return paragraphDocument("Session goal", noneLabel, []string{"No autonomous goal is active or paused for this session."})
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
	if current.Reason != nil {
		reason := string(current.Reason.Code)
		if current.Reason.RunOutcome != "" {
			reason += " · run " + string(current.Reason.RunOutcome)
		}
		if current.Reason.Explanation != "" {
			reason += format.Separator + current.Reason.Explanation
		}
		lines = append(lines, "reason     "+reason)
	}
	return paragraphDocument("Session goal", string(current.Status), lines)
}

func goalCostLabel(used protocol.GoalUsage) string {
	if used.CostUSD != nil {
		return format.USD(*used.CostUSD)
	}
	if used.Runs == 0 {
		return "no cost yet"
	}
	return costUnavailable
}

func (g *goalScreens) StartGoal(objective string) error {
	start := protocol.StartGoalRequest{
		SessionID: g.session.current.ID, Objective: objective,
		Provider: g.options.Provider, Model: g.options.Model, ReasoningEffort: g.options.ReasoningEffort,
	}
	if selected, ok := g.selectedGoal(); ok {
		start.ReplaceIncarnationID = selected.IncarnationID
	}
	return g.changeGoal("starting session goal", "goal · start accepted", func(ctx context.Context) (protocol.Goal, error) {
		return g.service.StartGoal(ctx, start)
	})
}

func (g *goalScreens) UpdateGoal(objective string) error {
	target, ok := g.selectedGoal()
	if !ok {
		return errNoDisplayedGoal
	}
	update := protocol.UpdateGoalRequest{GoalTarget: target, Objective: objective}
	return g.changeGoal("updating session goal", "goal · update accepted", func(ctx context.Context) (protocol.Goal, error) {
		return g.service.UpdateGoal(ctx, update)
	})
}

func (g *goalScreens) ClearGoal() error {
	return g.acknowledgeGoalChange("clearing session goal", "goal · cleared", g.service.ClearGoal)
}

func (g *goalScreens) StopGoal() error {
	return g.acknowledgeGoalChange("stopping session goal", "goal · stop accepted", g.service.StopGoal)
}

var errNoDisplayedGoal = errors.New("no Goal is displayed for the current session")

func (g *goalScreens) selectedGoal() (protocol.GoalTarget, bool) {
	current := g.header.goal
	if current == nil || current.SessionID != g.session.current.ID {
		return protocol.GoalTarget{}, false
	}
	return current.GoalTarget, true
}

func (g *goalScreens) acknowledgeGoalChange(label, accepted string, change func(context.Context, protocol.GoalTarget) error) error {
	target, ok := g.selectedGoal()
	if !ok {
		return errNoDisplayedGoal
	}
	presentation := g.session.context
	g.status.note(label)
	started := g.tasks.admissionMutation(goalOperation,
		func(ctx context.Context) (struct{}, error) {
			return struct{}{}, change(ctx, target)
		},
		func(_ struct{}, err error) {
			if err != nil {
				g.message(label + " failed: " + err.Error())
				return
			}
			if g.session.context.current(presentation) {
				g.ShowGoal()
			}
			g.status.note(accepted)
		},
	)
	if !started {
		return goalOperation.busy()
	}
	return nil
}

func (g *goalScreens) ResumeGoal() error {
	target, ok := g.selectedGoal()
	if !ok {
		return errNoDisplayedGoal
	}
	return g.changeGoal("resuming session goal", "goal · resume accepted", func(ctx context.Context) (protocol.Goal, error) {
		return g.service.ResumeGoal(ctx, target)
	})
}

func (g *goalScreens) changeGoal(label, accepted string, change func(context.Context) (protocol.Goal, error)) error {
	presentation := g.session.context
	g.status.note(label)
	started := g.tasks.admissionMutation(goalOperation, change, func(_ protocol.Goal, err error) {
		if err != nil {
			g.message(label + " failed: " + err.Error())
			return
		}
		if g.session.context.current(presentation) {
			g.ShowGoal()
		}
		g.status.note(accepted)
	})
	if !started {
		return goalOperation.busy()
	}
	return nil
}
