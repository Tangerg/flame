// Goal values describe the autonomous objective lifecycle observed and
// controlled within an agent Session.
package agent

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

type GoalStatus string

const (
	GoalActive     GoalStatus = "active"
	GoalPaused     GoalStatus = "paused"
	GoalBlocked    GoalStatus = "blocked"
	GoalCompleting GoalStatus = "completing"
)

func (s GoalStatus) valid() bool {
	return s == GoalActive || s == GoalPaused || s == GoalBlocked || s == GoalCompleting
}

// AllowsLifecycleCommands reports whether a start, stop, or resume request can
// be meaningful in this observed state. The runtime remains authoritative for
// concurrent transitions between the observation and a command.
func (s GoalStatus) AllowsLifecycleCommands() bool { return s.valid() && s != GoalCompleting }

type GoalReasonCode string

const (
	GoalReasonNone             GoalReasonCode = ""
	GoalStoppedByUser          GoalReasonCode = "stoppedByUser"
	GoalRuntimeRestarted       GoalReasonCode = "runtimeRestarted"
	GoalRunStartFailed         GoalReasonCode = "runStartFailed"
	GoalAwaitingInput          GoalReasonCode = "awaitingInput"
	GoalTerminalOutcomeMissing GoalReasonCode = "terminalOutcomeMissing"
	GoalRunNotCompleted        GoalReasonCode = "runNotCompleted"
	GoalRunBudgetReached       GoalReasonCode = "runBudgetReached"
	GoalCostBudgetReached      GoalReasonCode = "costBudgetReached"
	GoalStepBudgetReached      GoalReasonCode = "stepBudgetReached"
	GoalPricingUnavailable     GoalReasonCode = "pricingUnavailable"
	GoalBlockedByModel         GoalReasonCode = "blockedByModel"
)

func (r GoalReasonCode) valid() bool {
	switch r {
	case GoalStoppedByUser, GoalRuntimeRestarted, GoalRunStartFailed, GoalAwaitingInput,
		GoalTerminalOutcomeMissing, GoalRunNotCompleted, GoalRunBudgetReached,
		GoalCostBudgetReached, GoalStepBudgetReached, GoalPricingUnavailable, GoalBlockedByModel:
		return true
	default:
		return false
	}
}

type GoalReason struct {
	code   GoalReasonCode
	detail string
}

func (r GoalReason) Code() GoalReasonCode { return r.code }

func (r GoalReason) Detail() string { return r.detail }

func newGoalReason(status GoalStatus, code GoalReasonCode, detail string) (GoalReason, error) {
	if !code.valid() {
		return GoalReason{}, fmt.Errorf("goal reason %q is invalid", code)
	}
	if detail != strings.TrimSpace(detail) {
		return GoalReason{}, errors.New("goal reason detail must not have surrounding whitespace")
	}
	switch status {
	case GoalPaused:
		switch code {
		case GoalStoppedByUser, GoalRuntimeRestarted, GoalRunStartFailed, GoalAwaitingInput, GoalTerminalOutcomeMissing:
			if detail != "" {
				return GoalReason{}, fmt.Errorf("goal reason %q must not carry detail", code)
			}
		case GoalRunNotCompleted:
			if detail == "" {
				return GoalReason{}, errors.New("goal run-not-completed reason requires a terminal outcome")
			}
		default:
			return GoalReason{}, fmt.Errorf("goal reason %q cannot pause a goal", code)
		}
	case GoalBlocked:
		switch code {
		case GoalRunBudgetReached, GoalCostBudgetReached, GoalStepBudgetReached, GoalPricingUnavailable:
			if detail != "" {
				return GoalReason{}, fmt.Errorf("goal reason %q must not carry detail", code)
			}
		case GoalBlockedByModel:
			if detail == "" {
				return GoalReason{}, errors.New("goal model block requires an explanation")
			}
		default:
			return GoalReason{}, fmt.Errorf("goal reason %q cannot block a goal", code)
		}
	default:
		return GoalReason{}, fmt.Errorf("goal status %q cannot carry a reason", status)
	}
	return GoalReason{code: code, detail: detail}, nil
}

type GoalBudget struct {
	maxRuns      int
	maxCostUSD   float64
	maxSteps     int
	runsLimited  bool
	costLimited  bool
	stepsLimited bool
	initialized  bool
}

type GoalBudgetLimits struct {
	MaxRuns    *int
	MaxCostUSD *float64
	MaxSteps   *int
}

func UnlimitedGoalBudget() GoalBudget { return GoalBudget{initialized: true} }

func NewGoalBudget(limits GoalBudgetLimits) (GoalBudget, error) {
	budget := GoalBudget{initialized: true}
	if limits.MaxRuns != nil {
		if *limits.MaxRuns <= 0 {
			return GoalBudget{}, errors.New("goal budget maximum runs must be positive")
		}
		budget.maxRuns, budget.runsLimited = *limits.MaxRuns, true
	}
	if limits.MaxCostUSD != nil {
		if *limits.MaxCostUSD <= 0 || math.IsNaN(*limits.MaxCostUSD) || math.IsInf(*limits.MaxCostUSD, 0) {
			return GoalBudget{}, errors.New("goal budget maximum cost must be finite and positive")
		}
		budget.maxCostUSD, budget.costLimited = *limits.MaxCostUSD, true
	}
	if limits.MaxSteps != nil {
		if *limits.MaxSteps <= 0 {
			return GoalBudget{}, errors.New("goal budget maximum steps must be positive")
		}
		budget.maxSteps, budget.stepsLimited = *limits.MaxSteps, true
	}
	if budget.Unlimited() {
		return GoalBudget{}, errors.New("limited goal budget requires at least one limit")
	}
	return budget, nil
}

func (b GoalBudget) Validate() error {
	if !b.initialized {
		return errors.New("goal budget must be constructed explicitly")
	}
	if b.maxRuns < 0 || b.maxSteps < 0 ||
		b.runsLimited != (b.maxRuns > 0) || b.stepsLimited != (b.maxSteps > 0) {
		return errors.New("goal budget count limit presence and value disagree")
	}
	if b.maxCostUSD < 0 || b.costLimited != (b.maxCostUSD > 0) ||
		math.IsNaN(b.maxCostUSD) || math.IsInf(b.maxCostUSD, 0) {
		return errors.New("goal budget cost limit presence and value disagree")
	}
	return nil
}

func (b GoalBudget) Unlimited() bool {
	return b.initialized && !b.runsLimited && !b.costLimited && !b.stepsLimited
}

func (b GoalBudget) MaxRuns() (int, bool)        { return b.maxRuns, b.runsLimited }
func (b GoalBudget) MaxCostUSD() (float64, bool) { return b.maxCostUSD, b.costLimited }
func (b GoalBudget) MaxSteps() (int, bool)       { return b.maxSteps, b.stepsLimited }

func (b GoalBudget) exhausted(used GoalUsage) bool {
	return b.runsLimited && used.runs >= b.maxRuns ||
		b.costLimited && used.costUSD >= b.maxCostUSD ||
		b.stepsLimited && used.steps >= b.maxSteps
}

type GoalUsage struct {
	runs    int
	costUSD float64
	steps   int
}

func NewGoalUsage(runs int, costUSD float64, steps int) (GoalUsage, error) {
	usage := GoalUsage{runs: runs, costUSD: costUSD, steps: steps}
	if err := usage.Validate(); err != nil {
		return GoalUsage{}, err
	}
	return usage, nil
}

func (u GoalUsage) Validate() error {
	if u.runs < 0 || u.costUSD < 0 || u.steps < 0 ||
		math.IsNaN(u.costUSD) || math.IsInf(u.costUSD, 0) {
		return errors.New("goal usage contains a non-finite or negative value")
	}
	return nil
}

func (u GoalUsage) Runs() int        { return u.runs }
func (u GoalUsage) CostUSD() float64 { return u.costUSD }
func (u GoalUsage) Steps() int       { return u.steps }
func (u GoalUsage) IsZero() bool     { return u == (GoalUsage{}) }

// GoalSnapshot is the technical reconstruction boundary for a Runtime Goal
// projection. It is not a mutation surface: RestoreGoal validates and owns every
// field before the value can enter the CLI domain.
type GoalSnapshot struct {
	SessionID       string
	Objective       string
	Status          GoalStatus
	ReasonCode      GoalReasonCode
	ReasonDetail    string
	Provider        string
	Model           string
	ReasoningEffort string
	Budget          GoalBudget
	Used            GoalUsage
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Goal struct {
	sessionID       string
	objective       string
	status          GoalStatus
	reason          *GoalReason
	provider        string
	model           string
	reasoningEffort string
	budget          GoalBudget
	used            GoalUsage
	createdAt       time.Time
	updatedAt       time.Time
}

func RestoreGoal(snapshot GoalSnapshot) (Goal, error) {
	value := Goal{
		sessionID: snapshot.SessionID, objective: snapshot.Objective, status: snapshot.Status,
		provider: snapshot.Provider, model: snapshot.Model, reasoningEffort: snapshot.ReasoningEffort,
		budget: snapshot.Budget, used: snapshot.Used,
		createdAt: canonicalGoalTime(snapshot.CreatedAt), updatedAt: canonicalGoalTime(snapshot.UpdatedAt),
	}
	if snapshot.ReasonCode != GoalReasonNone || snapshot.ReasonDetail != "" {
		reason, err := newGoalReason(snapshot.Status, snapshot.ReasonCode, snapshot.ReasonDetail)
		if err != nil {
			return Goal{}, err
		}
		value.reason = &reason
	}
	if err := value.Validate(); err != nil {
		return Goal{}, err
	}
	return value, nil
}

func (g Goal) Validate() error {
	var problems []error
	if err := runtimeprotocol.ValidateSessionID(g.sessionID); err != nil {
		problems = append(problems, err)
	}
	if strings.TrimSpace(g.objective) == "" {
		problems = append(problems, errors.New("objective is empty"))
	} else if g.objective != strings.TrimSpace(g.objective) {
		problems = append(problems, errors.New("objective has surrounding whitespace"))
	}
	if !g.status.valid() {
		problems = append(problems, fmt.Errorf("status %q is invalid", g.status))
	}
	if (g.status == GoalActive || g.status == GoalCompleting) && g.reason != nil {
		problems = append(problems, errors.New("non-resting goal carries a stopping reason"))
	}
	if (g.status == GoalPaused || g.status == GoalBlocked) && g.reason == nil {
		problems = append(problems, errors.New("resting goal has no reason"))
	}
	if g.reason != nil {
		validated, err := newGoalReason(g.status, g.reason.code, g.reason.detail)
		if err != nil || validated != *g.reason {
			problems = append(problems, err)
		}
	}
	if err := runtimeprotocol.ValidateModelSelection(g.provider, g.model, g.reasoningEffort); err != nil {
		problems = append(problems, err)
	}
	problems = append(problems, g.budget.Validate(), g.used.Validate())
	if g.createdAt.IsZero() || g.updatedAt.IsZero() {
		problems = append(problems, errors.New("creation and update times are required"))
	} else if g.updatedAt.Before(g.createdAt) {
		problems = append(problems, errors.New("update time precedes creation"))
	}
	if g.status == GoalActive && g.budget.exhausted(g.used) {
		problems = append(problems, errors.New("active goal has exhausted its budget"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("goal: %w", err)
	}
	return nil
}

func (g Goal) Snapshot() GoalSnapshot {
	snapshot := GoalSnapshot{
		SessionID: g.sessionID, Objective: g.objective, Status: g.status,
		Provider: g.provider, Model: g.model, ReasoningEffort: g.reasoningEffort,
		Budget: g.budget, Used: g.used,
		CreatedAt: g.createdAt, UpdatedAt: g.updatedAt,
	}
	if g.reason != nil {
		snapshot.ReasonCode, snapshot.ReasonDetail = g.reason.code, g.reason.detail
	}
	return snapshot
}

func (g Goal) SessionID() string       { return g.sessionID }
func (g Goal) Objective() string       { return g.objective }
func (g Goal) Status() GoalStatus      { return g.status }
func (g Goal) Provider() string        { return g.provider }
func (g Goal) Model() string           { return g.model }
func (g Goal) ReasoningEffort() string { return g.reasoningEffort }
func (g Goal) Budget() GoalBudget      { return g.budget }
func (g Goal) Used() GoalUsage         { return g.used }
func (g Goal) CreatedAt() time.Time    { return g.createdAt }
func (g Goal) UpdatedAt() time.Time    { return g.updatedAt }

func (g Goal) Reason() (GoalReason, bool) {
	if g.reason == nil {
		return GoalReason{}, false
	}
	return *g.reason, true
}

func canonicalGoalTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC()
}

type StartGoal struct {
	SessionID       string
	Objective       string
	Provider        string
	Model           string
	ReasoningEffort string
	Budget          GoalBudget
}

// UpdateGoal revises the objective of the current Goal without replacing its
// lifecycle, model selection, budget, or accumulated usage.
type UpdateGoal struct {
	SessionID string
	Objective string
}

func (u UpdateGoal) Validate() error {
	if err := runtimeprotocol.ValidateSessionID(u.SessionID); err != nil {
		return fmt.Errorf("update goal: %w", err)
	}
	if strings.TrimSpace(u.Objective) == "" {
		return errors.New("update goal: objective is empty")
	}
	if u.Objective != strings.TrimSpace(u.Objective) {
		return errors.New("update goal: objective must not have surrounding whitespace")
	}
	return nil
}

func (u UpdateGoal) ValidateResult(result Goal) error {
	if err := u.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.SessionID() != u.SessionID {
		problems = append(problems, fmt.Errorf("runtime returned session %q, want %q", result.SessionID(), u.SessionID))
	}
	if result.Objective() != u.Objective {
		problems = append(problems, fmt.Errorf("runtime returned objective %q, want %q", result.Objective(), u.Objective))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("update goal: %w", err)
	}
	return nil
}

func (s StartGoal) Validate() error {
	if err := runtimeprotocol.ValidateSessionID(s.SessionID); err != nil {
		return fmt.Errorf("start goal: %w", err)
	}
	if strings.TrimSpace(s.Objective) == "" {
		return errors.New("start goal: objective is empty")
	}
	if s.Objective != strings.TrimSpace(s.Objective) {
		return errors.New("start goal: objective must not have surrounding whitespace")
	}
	if err := runtimeprotocol.ValidateModelSelection(s.Provider, s.Model, s.ReasoningEffort); err != nil {
		return fmt.Errorf("start goal: %w", err)
	}
	if err := s.Budget.Validate(); err != nil {
		return fmt.Errorf("start goal: %w", err)
	}
	return nil
}

// ValidateResult verifies that a successful start acknowledgement represents
// the fresh objective incarnation requested by the caller.
func (s StartGoal) ValidateResult(result Goal) error {
	if err := s.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.SessionID() != s.SessionID {
		problems = append(problems, fmt.Errorf("runtime returned session %q, want %q", result.SessionID(), s.SessionID))
	}
	if result.Objective() != s.Objective {
		problems = append(problems, fmt.Errorf("runtime returned objective %q, want %q", result.Objective(), s.Objective))
	}
	if result.Status() != GoalActive {
		problems = append(problems, fmt.Errorf("runtime returned status %q, want %q", result.Status(), GoalActive))
	}
	if result.Provider() != s.Provider || result.Model() != s.Model || result.ReasoningEffort() != s.ReasoningEffort {
		problems = append(problems, fmt.Errorf(
			"runtime returned model selection %q/%q/%q, want %q/%q/%q",
			result.Provider(), result.Model(), result.ReasoningEffort(),
			s.Provider, s.Model, s.ReasoningEffort,
		))
	}
	if result.Budget() != s.Budget {
		problems = append(problems, fmt.Errorf("runtime returned budget %+v, want %+v", result.Budget(), s.Budget))
	}
	if !result.Used().IsZero() {
		problems = append(problems, fmt.Errorf("runtime returned non-zero usage %+v for a new goal", result.Used()))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("start goal: %w", err)
	}
	return nil
}
