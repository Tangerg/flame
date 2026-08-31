// Package goal defines autonomous-session objective lifecycle values and its
// consumer-owned runtime port.
package goal

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	cliidentity "github.com/Tangerg/flame/cli/internal/identity"
)

type Status string

const (
	Active     Status = "active"
	Paused     Status = "paused"
	Blocked    Status = "blocked"
	Completing Status = "completing"
)

func (s Status) valid() bool {
	return s == Active || s == Paused || s == Blocked || s == Completing
}

// AllowsLifecycleCommands reports whether a start, stop, or resume request can
// be meaningful in this observed state. The runtime remains authoritative for
// concurrent transitions between the observation and a command.
func (s Status) AllowsLifecycleCommands() bool { return s.valid() && s != Completing }

type ReasonCode string

const (
	ReasonNone             ReasonCode = ""
	StoppedByUser          ReasonCode = "stoppedByUser"
	RuntimeRestarted       ReasonCode = "runtimeRestarted"
	RunStartFailed         ReasonCode = "runStartFailed"
	AwaitingInput          ReasonCode = "awaitingInput"
	TerminalOutcomeMissing ReasonCode = "terminalOutcomeMissing"
	RunNotCompleted        ReasonCode = "runNotCompleted"
	RunBudgetReached       ReasonCode = "runBudgetReached"
	CostBudgetReached      ReasonCode = "costBudgetReached"
	StepBudgetReached      ReasonCode = "stepBudgetReached"
	BlockedByModel         ReasonCode = "blockedByModel"
)

func (r ReasonCode) valid() bool {
	switch r {
	case StoppedByUser, RuntimeRestarted, RunStartFailed, AwaitingInput,
		TerminalOutcomeMissing, RunNotCompleted, RunBudgetReached,
		CostBudgetReached, StepBudgetReached, BlockedByModel:
		return true
	default:
		return false
	}
}

type Reason struct {
	code   ReasonCode
	detail string
}

func (r Reason) Code() ReasonCode { return r.code }

func (r Reason) Detail() string { return r.detail }

func newReason(status Status, code ReasonCode, detail string) (Reason, error) {
	if !code.valid() {
		return Reason{}, fmt.Errorf("goal reason %q is invalid", code)
	}
	if detail != strings.TrimSpace(detail) {
		return Reason{}, errors.New("goal reason detail must not have surrounding whitespace")
	}
	switch status {
	case Paused:
		switch code {
		case StoppedByUser, RuntimeRestarted, RunStartFailed, AwaitingInput, TerminalOutcomeMissing:
			if detail != "" {
				return Reason{}, fmt.Errorf("goal reason %q must not carry detail", code)
			}
		case RunNotCompleted:
			if detail == "" {
				return Reason{}, errors.New("goal run-not-completed reason requires a terminal outcome")
			}
		default:
			return Reason{}, fmt.Errorf("goal reason %q cannot pause a goal", code)
		}
	case Blocked:
		switch code {
		case RunBudgetReached, CostBudgetReached, StepBudgetReached:
			if detail != "" {
				return Reason{}, fmt.Errorf("goal reason %q must not carry detail", code)
			}
		case BlockedByModel:
			if detail == "" {
				return Reason{}, errors.New("goal model block requires an explanation")
			}
		default:
			return Reason{}, fmt.Errorf("goal reason %q cannot block a goal", code)
		}
	default:
		return Reason{}, fmt.Errorf("goal status %q cannot carry a reason", status)
	}
	return Reason{code: code, detail: detail}, nil
}

type Budget struct {
	maxRuns      int
	maxCostUSD   float64
	maxSteps     int
	runsLimited  bool
	costLimited  bool
	stepsLimited bool
	initialized  bool
}

type BudgetLimits struct {
	MaxRuns    *int
	MaxCostUSD *float64
	MaxSteps   *int
}

func UnlimitedBudget() Budget { return Budget{initialized: true} }

func NewBudget(limits BudgetLimits) (Budget, error) {
	budget := Budget{initialized: true}
	if limits.MaxRuns != nil {
		if *limits.MaxRuns <= 0 {
			return Budget{}, errors.New("goal budget maximum runs must be positive")
		}
		budget.maxRuns, budget.runsLimited = *limits.MaxRuns, true
	}
	if limits.MaxCostUSD != nil {
		if *limits.MaxCostUSD <= 0 || math.IsNaN(*limits.MaxCostUSD) || math.IsInf(*limits.MaxCostUSD, 0) {
			return Budget{}, errors.New("goal budget maximum cost must be finite and positive")
		}
		budget.maxCostUSD, budget.costLimited = *limits.MaxCostUSD, true
	}
	if limits.MaxSteps != nil {
		if *limits.MaxSteps <= 0 {
			return Budget{}, errors.New("goal budget maximum steps must be positive")
		}
		budget.maxSteps, budget.stepsLimited = *limits.MaxSteps, true
	}
	if budget.Unlimited() {
		return Budget{}, errors.New("limited goal budget requires at least one limit")
	}
	return budget, nil
}

func (b Budget) Validate() error {
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

func (b Budget) Unlimited() bool {
	return b.initialized && !b.runsLimited && !b.costLimited && !b.stepsLimited
}

func (b Budget) MaxRuns() (int, bool)        { return b.maxRuns, b.runsLimited }
func (b Budget) MaxCostUSD() (float64, bool) { return b.maxCostUSD, b.costLimited }
func (b Budget) MaxSteps() (int, bool)       { return b.maxSteps, b.stepsLimited }

func (b Budget) exhausted(used Usage) bool {
	return b.runsLimited && used.runs >= b.maxRuns ||
		b.costLimited && used.costUSD >= b.maxCostUSD ||
		b.stepsLimited && used.steps >= b.maxSteps
}

type Usage struct {
	runs    int
	costUSD float64
	steps   int
}

func NewUsage(runs int, costUSD float64, steps int) (Usage, error) {
	usage := Usage{runs: runs, costUSD: costUSD, steps: steps}
	if err := usage.Validate(); err != nil {
		return Usage{}, err
	}
	return usage, nil
}

func (u Usage) Validate() error {
	if u.runs < 0 || u.costUSD < 0 || u.steps < 0 ||
		math.IsNaN(u.costUSD) || math.IsInf(u.costUSD, 0) {
		return errors.New("goal usage contains a non-finite or negative value")
	}
	return nil
}

func (u Usage) Runs() int        { return u.runs }
func (u Usage) CostUSD() float64 { return u.costUSD }
func (u Usage) Steps() int       { return u.steps }
func (u Usage) IsZero() bool     { return u == (Usage{}) }

// Snapshot is the technical reconstruction boundary for a Runtime Goal
// projection. It is not a mutation surface: Restore validates and owns every
// field before the value can enter the CLI domain.
type Snapshot struct {
	SessionID    string
	Objective    string
	Status       Status
	ReasonCode   ReasonCode
	ReasonDetail string
	Provider     string
	Model        string
	Budget       Budget
	Used         Usage
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Goal struct {
	sessionID string
	objective string
	status    Status
	reason    *Reason
	provider  string
	model     string
	budget    Budget
	used      Usage
	createdAt time.Time
	updatedAt time.Time
}

func Restore(snapshot Snapshot) (Goal, error) {
	value := Goal{
		sessionID: snapshot.SessionID, objective: snapshot.Objective, status: snapshot.Status,
		provider: snapshot.Provider, model: snapshot.Model, budget: snapshot.Budget, used: snapshot.Used,
		createdAt: canonicalTime(snapshot.CreatedAt), updatedAt: canonicalTime(snapshot.UpdatedAt),
	}
	if snapshot.ReasonCode != ReasonNone || snapshot.ReasonDetail != "" {
		reason, err := newReason(snapshot.Status, snapshot.ReasonCode, snapshot.ReasonDetail)
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
	if err := cliidentity.ValidateSession(g.sessionID); err != nil {
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
	if (g.status == Active || g.status == Completing) && g.reason != nil {
		problems = append(problems, errors.New("non-resting goal carries a stopping reason"))
	}
	if (g.status == Paused || g.status == Blocked) && g.reason == nil {
		problems = append(problems, errors.New("resting goal has no reason"))
	}
	if g.reason != nil {
		validated, err := newReason(g.status, g.reason.code, g.reason.detail)
		if err != nil || validated != *g.reason {
			problems = append(problems, err)
		}
	}
	if err := cliidentity.ValidateModelSelection(g.provider, g.model, ""); err != nil {
		problems = append(problems, err)
	}
	problems = append(problems, g.budget.Validate(), g.used.Validate())
	if g.createdAt.IsZero() || g.updatedAt.IsZero() {
		problems = append(problems, errors.New("creation and update times are required"))
	} else if g.updatedAt.Before(g.createdAt) {
		problems = append(problems, errors.New("update time precedes creation"))
	}
	if g.status == Active && g.budget.exhausted(g.used) {
		problems = append(problems, errors.New("active goal has exhausted its budget"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("goal: %w", err)
	}
	return nil
}

func (g Goal) Snapshot() Snapshot {
	snapshot := Snapshot{
		SessionID: g.sessionID, Objective: g.objective, Status: g.status,
		Provider: g.provider, Model: g.model, Budget: g.budget, Used: g.used,
		CreatedAt: g.createdAt, UpdatedAt: g.updatedAt,
	}
	if g.reason != nil {
		snapshot.ReasonCode, snapshot.ReasonDetail = g.reason.code, g.reason.detail
	}
	return snapshot
}

func (g Goal) SessionID() string    { return g.sessionID }
func (g Goal) Objective() string    { return g.objective }
func (g Goal) Status() Status       { return g.status }
func (g Goal) Provider() string     { return g.provider }
func (g Goal) Model() string        { return g.model }
func (g Goal) Budget() Budget       { return g.budget }
func (g Goal) Used() Usage          { return g.used }
func (g Goal) CreatedAt() time.Time { return g.createdAt }
func (g Goal) UpdatedAt() time.Time { return g.updatedAt }

func (g Goal) Reason() (Reason, bool) {
	if g.reason == nil {
		return Reason{}, false
	}
	return *g.reason, true
}

func canonicalTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC()
}

type Start struct {
	SessionID string
	Objective string
	Provider  string
	Model     string
	Budget    Budget
}

// Update revises the objective of the current Goal without replacing its
// lifecycle, model selection, budget, or accumulated usage.
type Update struct {
	SessionID string
	Objective string
}

func (u Update) Validate() error {
	if err := cliidentity.ValidateSession(u.SessionID); err != nil {
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

func (u Update) ValidateResult(result Goal) error {
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

func (s Start) Validate() error {
	if err := cliidentity.ValidateSession(s.SessionID); err != nil {
		return fmt.Errorf("start goal: %w", err)
	}
	if strings.TrimSpace(s.Objective) == "" {
		return errors.New("start goal: objective is empty")
	}
	if s.Objective != strings.TrimSpace(s.Objective) {
		return errors.New("start goal: objective must not have surrounding whitespace")
	}
	if err := cliidentity.ValidateModelSelection(s.Provider, s.Model, ""); err != nil {
		return fmt.Errorf("start goal: %w", err)
	}
	if err := s.Budget.Validate(); err != nil {
		return fmt.Errorf("start goal: %w", err)
	}
	return nil
}

// ValidateResult verifies that a successful start acknowledgement represents
// the fresh objective incarnation requested by the caller.
func (s Start) ValidateResult(result Goal) error {
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
	if result.Status() != Active {
		problems = append(problems, fmt.Errorf("runtime returned status %q, want %q", result.Status(), Active))
	}
	if result.Provider() != s.Provider || result.Model() != s.Model {
		problems = append(problems, fmt.Errorf(
			"runtime returned model %q/%q, want %q/%q",
			result.Provider(), result.Model(), s.Provider, s.Model,
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

type Service interface {
	GetGoal(context.Context, string) (Goal, bool, error)
	StartGoal(context.Context, Start) (Goal, error)
	UpdateGoal(context.Context, Update) (Goal, error)
	ClearGoal(context.Context, string) error
	StopGoal(context.Context, string) (Goal, error)
	ResumeGoal(context.Context, string) (Goal, error)
}
