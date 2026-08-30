package run

import (
	"errors"
	"math"
)

// Limits is the immutable accumulated allowance a Run may consume before it is
// stopped. Its useful zero value is an unlimited policy; callers inspect limit
// presence through the accessors rather than comparing numeric sentinels.
//
// It lives beside [State] and [Outcome] rather than with accrued accounting:
// admission fixes policy, the executor enforces it, and restart recovery reapplies
// the exact same value. What was actually spent remains a recorded fact.
type Limits struct {
	maxTotalTokens int64
	maxSteps       int
	maxBudgetUSD   float64
	tokensLimited  bool
	stepsLimited   bool
	budgetLimited  bool
}

// LimitValues is the construction boundary for a limited policy. Every present
// value is a real positive cap; absence means that dimension is not capped. At
// least one dimension must be present.
type LimitValues struct {
	MaxTotalTokens *int64
	MaxSteps       *int
	MaxBudgetUSD   *float64
}

// UnlimitedLimits returns a Run policy with no accumulated allowance cap.
func UnlimitedLimits() Limits { return Limits{} }

// NewLimits constructs a policy with at least one explicit positive limit.
func NewLimits(values LimitValues) (Limits, error) {
	var limits Limits
	if values.MaxTotalTokens != nil {
		if *values.MaxTotalTokens <= 0 {
			return Limits{}, errors.New("run: max total tokens must be positive")
		}
		limits.maxTotalTokens, limits.tokensLimited = *values.MaxTotalTokens, true
	}
	if values.MaxSteps != nil {
		if *values.MaxSteps <= 0 {
			return Limits{}, errors.New("run: max steps must be positive")
		}
		limits.maxSteps, limits.stepsLimited = *values.MaxSteps, true
	}
	if values.MaxBudgetUSD != nil {
		if *values.MaxBudgetUSD <= 0 || math.IsNaN(*values.MaxBudgetUSD) || math.IsInf(*values.MaxBudgetUSD, 0) {
			return Limits{}, errors.New("run: max budget USD must be finite and positive")
		}
		limits.maxBudgetUSD, limits.budgetLimited = *values.MaxBudgetUSD, true
	}
	if limits.Unlimited() {
		return Limits{}, errors.New("run: limited policy requires at least one limit")
	}
	return limits, nil
}

// Validate verifies the private presence/value pairs. The check deliberately
// remains on the value because persistence restores data from outside Domain.
func (l Limits) Validate() error {
	if l.maxTotalTokens < 0 || l.maxSteps < 0 ||
		l.tokensLimited != (l.maxTotalTokens > 0) || l.stepsLimited != (l.maxSteps > 0) {
		return errors.New("run: count limit presence and value disagree")
	}
	if l.maxBudgetUSD < 0 || l.budgetLimited != (l.maxBudgetUSD > 0) ||
		math.IsNaN(l.maxBudgetUSD) || math.IsInf(l.maxBudgetUSD, 0) {
		return errors.New("run: budget limit presence and value disagree")
	}
	return nil
}

// Unlimited reports whether no accumulated allowance dimension is capped.
func (l Limits) Unlimited() bool {
	return !l.tokensLimited && !l.stepsLimited && !l.budgetLimited
}

func (l Limits) MaxTotalTokens() (int64, bool) { return l.maxTotalTokens, l.tokensLimited }
func (l Limits) MaxSteps() (int, bool)         { return l.maxSteps, l.stepsLimited }
func (l Limits) MaxBudgetUSD() (float64, bool) { return l.maxBudgetUSD, l.budgetLimited }
