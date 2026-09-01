package goal

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
)

// Budget is the immutable cross-Run spending policy. Its zero value is invalid;
// callers must choose [UnlimitedBudget] or construct explicit positive limits
// with [NewBudget].
type Budget struct {
	maxRuns      int
	maxCostUSD   float64
	maxSteps     int
	runsLimited  bool
	costLimited  bool
	stepsLimited bool
	initialized  bool
}

// BudgetLimits is the construction boundary for a limited Budget. Every
// present value is a real positive cap; absence means that axis is not capped.
// At least one axis must be present.
type BudgetLimits struct {
	MaxRuns    *int
	MaxCostUSD *float64
	MaxSteps   *int
}

// UnlimitedBudget explicitly selects a Goal with no cross-Run cap.
func UnlimitedBudget() Budget {
	return Budget{initialized: true}
}

// NewBudget constructs a Budget with at least one explicit positive limit.
func NewBudget(limits BudgetLimits) (Budget, error) {
	budget := Budget{initialized: true}
	if limits.MaxRuns != nil {
		if *limits.MaxRuns <= 0 {
			return Budget{}, fmt.Errorf("%w: maximum runs must be positive", ErrInvalid)
		}
		budget.maxRuns, budget.runsLimited = *limits.MaxRuns, true
	}
	if limits.MaxCostUSD != nil {
		if *limits.MaxCostUSD <= 0 || math.IsNaN(*limits.MaxCostUSD) || math.IsInf(*limits.MaxCostUSD, 0) {
			return Budget{}, fmt.Errorf("%w: maximum cost must be finite and positive", ErrInvalid)
		}
		budget.maxCostUSD, budget.costLimited = *limits.MaxCostUSD, true
	}
	if limits.MaxSteps != nil {
		if *limits.MaxSteps <= 0 {
			return Budget{}, fmt.Errorf("%w: maximum steps must be positive", ErrInvalid)
		}
		budget.maxSteps, budget.stepsLimited = *limits.MaxSteps, true
	}
	if budget.Unlimited() {
		return Budget{}, fmt.Errorf("%w: limited budget requires at least one limit", ErrInvalid)
	}
	return budget, nil
}

func (b Budget) Validate() error {
	if !b.initialized {
		return fmt.Errorf("%w: budget must be constructed explicitly", ErrInvalid)
	}
	if b.maxRuns < 0 || b.maxSteps < 0 ||
		b.runsLimited != (b.maxRuns > 0) || b.stepsLimited != (b.maxSteps > 0) {
		return fmt.Errorf("%w: count limit presence and value disagree", ErrInvalid)
	}
	if b.maxCostUSD < 0 || b.costLimited != (b.maxCostUSD > 0) ||
		math.IsNaN(b.maxCostUSD) || math.IsInf(b.maxCostUSD, 0) {
		return fmt.Errorf("%w: cost limit presence and value disagree", ErrInvalid)
	}
	return nil
}

// Unlimited reports whether no budget axis is capped.
func (b Budget) Unlimited() bool {
	return b.initialized && !b.runsLimited && !b.costLimited && !b.stepsLimited
}

func (b Budget) MaxRuns() (int, bool)        { return b.maxRuns, b.runsLimited }
func (b Budget) MaxCostUSD() (float64, bool) { return b.maxCostUSD, b.costLimited }
func (b Budget) MaxSteps() (int, bool)       { return b.maxSteps, b.stepsLimited }

// Usage is the immutable accounting value accumulated across Goal-owned Runs.
type Usage struct {
	Runs  int
	Cost  accounting.Cost
	Steps int
}

func (u Usage) validate() error {
	if u.Runs < 0 || u.Steps < 0 {
		return errors.New("goal: usage counts must be non-negative")
	}
	if err := u.Cost.Validate(); err != nil {
		return fmt.Errorf("goal: usage cost: %w", err)
	}
	if u.Runs == 0 && (u.Steps != 0 || u.Cost != (accounting.Cost{})) {
		return errors.New("goal: empty usage carries spending")
	}
	return nil
}

func (u Usage) add(record RunRecord) (Usage, error) {
	if err := u.validate(); err != nil {
		return Usage{}, err
	}
	if u.Runs == math.MaxInt || record.Steps > math.MaxInt-u.Steps {
		return Usage{}, errors.New("goal: usage counter overflow")
	}
	next := Usage{
		Runs:  u.Runs + 1,
		Cost:  record.Cost,
		Steps: u.Steps + record.Steps,
	}
	if u.Runs > 0 {
		var err error
		next.Cost, err = u.Cost.Add(record.Cost)
		if err != nil {
			return Usage{}, fmt.Errorf("goal: aggregate usage cost: %w", err)
		}
	}
	if err := next.validate(); err != nil {
		return Usage{}, err
	}
	return next, nil
}

type BudgetLimit string

const (
	BudgetLimitRuns  BudgetLimit = "runs"
	BudgetLimitCost  BudgetLimit = "cost"
	BudgetLimitSteps BudgetLimit = "steps"
)

func (b BudgetLimit) Valid() bool {
	return b == BudgetLimitRuns || b == BudgetLimitCost || b == BudgetLimitSteps
}

func (b Budget) exceeded(u Usage) (BudgetLimit, bool) {
	cost, priced := u.Cost.USD()
	switch {
	case b.runsLimited && u.Runs >= b.maxRuns:
		return BudgetLimitRuns, true
	case b.costLimited && priced && cost >= b.maxCostUSD:
		return BudgetLimitCost, true
	case b.stepsLimited && u.Steps >= b.maxSteps:
		return BudgetLimitSteps, true
	default:
		return BudgetLimit(""), false
	}
}

func (b Budget) pricingUnavailable(u Usage) bool {
	if !b.costLimited || u.Runs == 0 {
		return false
	}
	_, priced := u.Cost.USD()
	return !priced
}

type RunRecord struct {
	SessionID     string
	IncarnationID string
	RunID         string
	Outcome       run.Outcome
	Cost          accounting.Cost
	Steps         int
	CompletedAt   time.Time
}

func (r RunRecord) Validate() error {
	if err := validateSessionIdentity(r.SessionID); err != nil {
		return fmt.Errorf("goal: Run: %w", err)
	}
	if _, err := goalref.ParseIncarnation(r.IncarnationID); err != nil {
		return fmt.Errorf("%w: Run: %v", ErrInvalid, err)
	}
	if _, err := resourceid.ParseRun(r.RunID); err != nil {
		return fmt.Errorf("%w: Run ID: %v", ErrInvalid, err)
	}
	if _, ok := run.ParseOutcome(r.Outcome.String()); !ok {
		return fmt.Errorf("goal: Run has unknown outcome %q", r.Outcome)
	}
	if err := r.Cost.Validate(); err != nil {
		return fmt.Errorf("goal: Run cost: %w", err)
	}
	if r.Steps < 0 {
		return errors.New("goal: Run steps must not be negative")
	}
	if r.CompletedAt.IsZero() {
		return errors.New("goal: Run completion time is required")
	}
	return nil
}

// RecordRun returns one replacement revision even when accounting also derives
// a pause or budget block.
func (g Goal) RecordRun(record RunRecord) (Goal, error) {
	if err := record.Validate(); err != nil {
		return Goal{}, err
	}
	if record.SessionID != g.sessionID || record.IncarnationID != g.incarnationID.String() {
		return Goal{}, fmt.Errorf("%w: Run belongs to another Goal", ErrRunIdentityConflict)
	}
	next, err := g.next(record.CompletedAt)
	if err != nil {
		return Goal{}, err
	}
	next.used, err = g.used.add(record)
	if err != nil {
		return Goal{}, err
	}
	if g.status == StatusActive {
		if record.Outcome != run.OutcomeCompleted {
			next.status = StatusPaused
			next.reason, err = newReason(StatusPaused, ReasonRunNotCompleted, record.Outcome.String())
		} else if limit, exhausted := g.budget.exceeded(next.used); exhausted {
			next.status = StatusBlocked
			next.reason, err = newReason(StatusBlocked, reasonForBudgetLimit(limit), "")
		} else if g.budget.pricingUnavailable(next.used) {
			next.status = StatusBlocked
			next.reason, err = newReason(StatusBlocked, ReasonPricingUnavailable, "")
		}
		if err != nil {
			return Goal{}, err
		}
	}
	return next, next.ValidateSnapshot()
}

func reasonForBudgetLimit(limit BudgetLimit) ReasonCode {
	switch limit {
	case BudgetLimitRuns:
		return ReasonRunBudgetReached
	case BudgetLimitCost:
		return ReasonCostBudgetReached
	case BudgetLimitSteps:
		return ReasonStepBudgetReached
	default:
		panic("goal: impossible budget limit")
	}
}
