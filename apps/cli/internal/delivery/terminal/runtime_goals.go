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

// goalScreens shows and changes the current session's autonomous goal.
type goalScreens struct {
	screenHost
	service Goals
	// options are the session's current run options a started goal runs with.
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
	if current.Reason != nil {
		reason := string(current.Reason.Code)
		if current.Reason.RunOutcome != "" {
			reason += " · run " + string(current.Reason.RunOutcome)
		}
		if current.Reason.Explanation != "" {
			reason += " · " + current.Reason.Explanation
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
	return "cost unavailable"
}

func (g *goalScreens) StartGoal(objective string) error {
	start := protocol.StartGoalRequest{
		SessionID: g.session.current.ID, Objective: objective,
		Provider: g.options.Provider, Model: g.options.Model, ReasoningEffort: g.options.ReasoningEffort,
	}
	return g.changeGoal("starting session goal", func(ctx context.Context) (protocol.Goal, error) {
		return g.service.StartGoal(ctx, start)
	})
}

func (g *goalScreens) UpdateGoal(objective string) error {
	update := protocol.UpdateGoalRequest{SessionID: g.session.current.ID, Objective: objective}
	return g.changeGoal("updating session goal", func(ctx context.Context) (protocol.Goal, error) {
		return g.service.UpdateGoal(ctx, update)
	})
}

func (g *goalScreens) ClearGoal() error {
	presentation := g.session.context
	sessionID := g.session.current.ID
	label := "clearing session goal"
	g.status.note(label)
	started := g.tasks.admissionMutation(goalOperation, false,
		func(ctx context.Context) (struct{}, error) {
			return struct{}{}, g.service.ClearGoal(ctx, sessionID)
		},
		func(_ struct{}, err error) {
			if err != nil {
				g.message(label + " failed: " + err.Error())
				return
			}
			if g.session.context.current(presentation) {
				g.header.SetGoal(nil)
				g.reader.open(g.goalReaderQuery(), goalDocument(protocol.Goal{}, false))
			}
			g.status.note("goal · cleared")
		},
	)
	if !started {
		return errors.New("another goal operation is running")
	}
	return nil
}

func (g *goalScreens) StopGoal() error {
	sessionID := g.session.current.ID
	return g.changeGoal("stopping session goal", func(ctx context.Context) (protocol.Goal, error) {
		return g.service.StopGoal(ctx, sessionID)
	})
}

func (g *goalScreens) ResumeGoal() error {
	sessionID := g.session.current.ID
	return g.changeGoal("resuming session goal", func(ctx context.Context) (protocol.Goal, error) {
		return g.service.ResumeGoal(ctx, sessionID)
	})
}

func (g *goalScreens) changeGoal(label string, change func(context.Context) (protocol.Goal, error)) error {
	presentation := g.session.context
	g.status.note(label)
	started := g.tasks.admissionMutation(goalOperation, false, change, func(current protocol.Goal, err error) {
		if err != nil {
			g.message(label + " failed: " + err.Error())
			return
		}
		if g.session.context.current(presentation) {
			g.header.SetGoal(&current)
			g.reader.open(g.goalReaderQuery(), goalDocument(current, true))
		}
		g.status.note("goal · " + string(current.Status))
	})
	if !started {
		return errors.New("another goal operation is running")
	}
	return nil
}
