package builtin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/application/goals"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"

	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/samber/lo"
)

const createDescription = `Create and start a persistent autonomous Goal for the current session.

Call this only when the user explicitly asks for work to continue autonomously
across Runs. Do not infer Goal intent from an ordinary coding request; use
set_plan for the current request instead. The Goal starts after the current Run
releases the session and keeps launching Runs until it reports an outcome or
the user stops it. Copy the user's desired end state into objective.`

const getDescription = `Get the current session's autonomous Goal, including its objective, status,
stop reason, accumulated usage, model selection, and timestamps.
Returns goal=null when no Goal exists. Use this before creating a Goal when its
current state is uncertain.`

var reportDescription = fmt.Sprintf(`Report the terminal outcome of the autonomous Goal you are currently pursuing.

Call this only from an autonomous Goal Run:
  - outcome="%s" only when the entire objective is achieved and verified;
  - outcome="%s" only when progress genuinely requires user input or an
    external state change, with a concrete reason.

Do not report completion for a plan, partial result, intention, or because one
Run ended. While no terminal outcome is reported, the Goal loop supplies the
next Run automatically.`, goals.OutcomeCompleted, goals.OutcomeBlocked)

type createArgs struct {
	Objective string `json:"objective" jsonschema:"required,minLength=1" jsonschema_description:"The complete end state to achieve autonomously, in natural language."`
}

type getArgs struct{}

type reportArgs struct {
	Outcome goals.Outcome `json:"outcome" jsonschema:"required" jsonschema_description:"The terminal outcome described by this Tool's reporting contract."`
	Reason  *string       `json:"reason,omitzero" jsonschema_description:"Concrete blocker and what must change. Required when reporting a blocker; omit when reporting completion."`
}

type GoalReader interface {
	Current(ctx context.Context, sessionID identity.SessionID) (goal.Goal, bool, error)
}

type GoalOutcomeReporter interface {
	Report(ctx context.Context, command goals.ReportCommand) (goals.ReportResult, error)
}

type goalStarter interface {
	Start(ctx context.Context, sessionID identity.SessionID, objective string, selection flamemodel.Selection, capabilities run.Capabilities) (goal.Goal, error)
}

type creator struct{ goals goalStarter }
type getter struct{ goals GoalReader }
type outcomeReporter struct{ goals GoalOutcomeReporter }

type goalResult struct {
	Goal    *goalView `json:"goal" jsonschema:"nullable"`
	Message string    `json:"message,omitempty"`
}

type goalView struct {
	SessionID       string      `json:"session_id"`
	Objective       string      `json:"objective"`
	Status          goal.Status `json:"status"`
	Reason          string      `json:"reason,omitempty"`
	Provider        string      `json:"provider,omitempty"`
	Model           string      `json:"model,omitempty"`
	ReasoningEffort string      `json:"reasoning_effort,omitempty"`

	Usage     usageView `json:"usage"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type usageView struct {
	Runs    int      `json:"runs"`
	CostUSD *float64 `json:"cost_usd,omitzero"`
	Steps   int      `json:"steps"`
}

// Assembled after the Goal Driver because starting a Goal consumes the later-built
// Run coordinator.
func NewCreate(starter goalStarter) (toolcontract.Tool, error) {
	if lo.IsNil(starter) {
		return nil, fmt.Errorf("%s: Goal starter is required", builtincontract.CreateGoal)
	}
	return toolcontract.NewFunc[createArgs, goalResult](
		toolcontract.FuncConfig{Name: string(builtincontract.CreateGoal), Description: createDescription},
		(&creator{goals: starter}).create,
	)
}

func NewGet(reader GoalReader) (toolcontract.Tool, error) {
	if lo.IsNil(reader) {
		return nil, fmt.Errorf("%s: Goal reader is required", builtincontract.GetGoal)
	}
	return toolcontract.NewFunc[getArgs, goalResult](
		toolcontract.FuncConfig{Name: string(builtincontract.GetGoal), Description: getDescription},
		(&getter{goals: reader}).get,
	)
}

func NewReport(reporter GoalOutcomeReporter) (toolcontract.Tool, error) {
	if lo.IsNil(reporter) {
		return nil, fmt.Errorf("%s: Goal outcome reporter is required", builtincontract.ReportGoalOutcome)
	}
	return toolcontract.NewFunc[reportArgs, string](
		toolcontract.FuncConfig{Name: string(builtincontract.ReportGoalOutcome), Description: reportDescription},
		(&outcomeReporter{goals: reporter}).report,
	)
}

func (c *creator) create(ctx context.Context, args createArgs) (goalResult, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID.IsZero() {
		return goalResult{Message: "No active session; a Goal must belong to a session."}, nil
	}
	if strings.TrimSpace(args.Objective) == "" {
		return goalResult{Message: "Provide a non-empty autonomous objective."}, nil
	}
	capabilities, _ := executionctx.RunCapabilities(ctx)
	// Nobody chose a model for this Goal, so its Runs follow the Session's
	// selection rather than a copy of the model this Run happens to use.
	g, err := c.goals.Start(ctx, sessionID, args.Objective, flamemodel.Selection{}, capabilities)
	if err != nil {
		switch {
		case errors.Is(err, goals.ErrGoalActive):
			return goalResult{Message: "An active Goal already exists for this session. Inspect it with get_goal before deciding what to do."}, nil
		case errors.Is(err, goals.ErrNoSession):
			return goalResult{Message: "The current session no longer exists; no Goal was created."}, nil
		case errors.Is(err, goals.ErrClosed):
			return goalResult{Message: "The service is shutting down; no Goal was created."}, nil
		case errors.Is(err, goals.ErrGoalConflict):
			return goalResult{Message: "The Goal changed concurrently. Inspect the current state with get_goal before retrying."}, nil
		default:
			return goalResult{}, err
		}
	}
	view := viewOf(g)
	return goalResult{
		Goal:    &view,
		Message: "Goal created. Its autonomous loop will begin after the current Run releases the session.",
	}, nil
}

func (g *getter) get(ctx context.Context, _ getArgs) (goalResult, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID.IsZero() {
		return goalResult{Message: "No active session; there is no session Goal to inspect."}, nil
	}
	current, ok, err := g.goals.Current(ctx, sessionID)
	if err != nil {
		return goalResult{}, pluginsdk.DefiniteToolFailure(fmt.Errorf("%s: %w", builtincontract.GetGoal, err))
	}
	if !ok {
		return goalResult{Message: "No Goal exists for this session."}, nil
	}
	view := viewOf(current)
	return goalResult{Goal: &view}, nil
}

func (o *outcomeReporter) report(ctx context.Context, args reportArgs) (string, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID.IsZero() {
		return "No active session; cannot report a Goal outcome.", nil
	}
	incarnationID, goalRun := executionctx.GoalIncarnationID(ctx)
	if !goalRun {
		return "This Run is not pursuing a Goal; no outcome was reported.", nil
	}
	reason := ""
	if args.Reason != nil {
		reason = *args.Reason
	}
	result, err := o.goals.Report(ctx, goals.ReportCommand{
		SessionID:     sessionID,
		IncarnationID: incarnationID,
		Outcome:       args.Outcome,
		Reason:        reason,
	})
	if err != nil {
		return "", err
	}
	switch result {
	case goals.ReportApplied:
		if args.Outcome == goals.OutcomeCompleted {
			return "Goal outcome reported as completed. The autonomous loop will stop after this Run.", nil
		}
		return "Goal outcome reported as blocked. The loop will stop and surface the reason to the user.", nil
	case goals.ReportNoActiveGoal:
		return "No active Goal exists for this session; no outcome was reported.", nil
	case goals.ReportSuperseded:
		return "This Run belongs to a superseded Goal; no outcome was reported.", nil
	case goals.ReportConflict:
		return "The Goal changed concurrently; inspect it with get_goal before reporting an outcome.", nil
	case goals.ReportReasonRequired:
		return "Provide a concrete reason when reporting a blocked Goal.", nil
	case goals.ReportReasonForbidden:
		return "Omit reason when reporting a completed Goal.", nil
	case goals.ReportInvalidOutcome:
		return "Invalid Goal outcome; use completed or blocked.", nil
	default:
		return "", errors.New("goal tool report result has no presentation")
	}
}

func viewOf(g goal.Goal) goalView {
	selection, used := g.ModelSelection(), g.Used()
	return goalView{
		SessionID:       g.SessionID().String(),
		Objective:       g.Objective(),
		Status:          g.Status(),
		Reason:          reasonText(g.Reason()),
		Provider:        selection.Provider(),
		Model:           selection.Model(),
		ReasoningEffort: selection.ReasoningEffort(),
		Usage: usageView{
			Runs:    used.Runs,
			CostUSD: used.Cost.OptionalUSD(),
			Steps:   used.Steps,
		},
		CreatedAt: g.CreatedAt(),
		UpdatedAt: g.UpdatedAt(),
	}
}

func reasonText(reason goal.Reason) string {
	switch reason.Code() {
	case goal.ReasonNone:
		return ""
	case goal.ReasonStoppedByUser:
		return "stopped by the user"
	case goal.ReasonRuntimeRestarted:
		return "the service restarted; resume to continue"
	case goal.ReasonRunStartFailed:
		return "could not start the next Run"
	case goal.ReasonAwaitingInput:
		return "the Run is waiting for user input"
	case goal.ReasonTerminalOutcomeMissing:
		return "the Run ended without a terminal outcome"
	case goal.ReasonRunNotCompleted:
		outcome, _ := reason.RunOutcome()
		return "the Run ended: " + string(outcome)
	case goal.ReasonBlockedByModel:
		explanation, _ := reason.Explanation()
		return explanation
	default:
		return "the Goal stopped"
	}
}
