package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
)

type runLimitsKind string

const (
	unlimitedRunLimits runLimitsKind = "unlimited"
	limitedRunLimits   runLimitsKind = "limited"
)

// RunLimits is the immutable accumulated allowance for one Run tree. Its zero
// value is invalid; unlimited and limited policies are both explicit.
type RunLimits struct {
	maxTotalTokens int64
	maxSteps       int
	maxBudgetUSD   float64
	tokensLimited  bool
	stepsLimited   bool
	budgetLimited  bool
	initialized    bool
}

// RunLimitValues is the construction boundary for a limited policy. Present
// values are real caps and therefore must be finite and strictly positive.
type RunLimitValues struct {
	MaxTotalTokens *int64
	MaxSteps       *int
	MaxBudgetUSD   *float64
}

func UnlimitedRunLimits() RunLimits { return RunLimits{initialized: true} }

func NewRunLimits(values RunLimitValues) (RunLimits, error) {
	limits := RunLimits{initialized: true}
	if values.MaxTotalTokens != nil {
		if *values.MaxTotalTokens <= 0 {
			return RunLimits{}, errors.New("run limits: max total tokens must be positive")
		}
		limits.maxTotalTokens, limits.tokensLimited = *values.MaxTotalTokens, true
	}
	if values.MaxSteps != nil {
		if *values.MaxSteps <= 0 {
			return RunLimits{}, errors.New("run limits: max steps must be positive")
		}
		limits.maxSteps, limits.stepsLimited = *values.MaxSteps, true
	}
	if values.MaxBudgetUSD != nil {
		if *values.MaxBudgetUSD <= 0 || math.IsNaN(*values.MaxBudgetUSD) || math.IsInf(*values.MaxBudgetUSD, 0) {
			return RunLimits{}, errors.New("run limits: max budget USD must be finite and positive")
		}
		limits.maxBudgetUSD, limits.budgetLimited = *values.MaxBudgetUSD, true
	}
	if limits.Unlimited() {
		return RunLimits{}, errors.New("run limits: limited policy requires at least one limit")
	}
	return limits, nil
}

func (r RunLimits) Validate() error {
	if !r.initialized {
		return errors.New("run limits: policy must be constructed explicitly")
	}
	if r.maxTotalTokens < 0 || r.maxSteps < 0 ||
		r.tokensLimited != (r.maxTotalTokens > 0) || r.stepsLimited != (r.maxSteps > 0) {
		return errors.New("run limits: count limit presence and value disagree")
	}
	if r.maxBudgetUSD < 0 || r.budgetLimited != (r.maxBudgetUSD > 0) ||
		math.IsNaN(r.maxBudgetUSD) || math.IsInf(r.maxBudgetUSD, 0) {
		return errors.New("run limits: budget limit presence and value disagree")
	}
	return nil
}

func (r RunLimits) Unlimited() bool {
	return r.initialized && !r.tokensLimited && !r.stepsLimited && !r.budgetLimited
}

func (r RunLimits) MaxTotalTokens() (int64, bool) { return r.maxTotalTokens, r.tokensLimited }
func (r RunLimits) MaxSteps() (int, bool)         { return r.maxSteps, r.stepsLimited }
func (r RunLimits) MaxBudgetUSD() (float64, bool) { return r.maxBudgetUSD, r.budgetLimited }

type runLimitsJSON struct {
	Kind           runLimitsKind `json:"type"`
	MaxTotalTokens *int64        `json:"maxTotalTokens,omitempty"`
	MaxSteps       *int          `json:"maxSteps,omitempty"`
	MaxBudgetUSD   *float64      `json:"maxBudgetUsd,omitempty"`
}

func (r RunLimits) MarshalJSON() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	wire := runLimitsJSON{Kind: unlimitedRunLimits}
	if !r.Unlimited() {
		wire.Kind = limitedRunLimits
		if value, present := r.MaxTotalTokens(); present {
			wire.MaxTotalTokens = &value
		}
		if value, present := r.MaxSteps(); present {
			wire.MaxSteps = &value
		}
		if value, present := r.MaxBudgetUSD(); present {
			wire.MaxBudgetUSD = &value
		}
	}
	return json.Marshal(wire)
}

func (r *RunLimits) UnmarshalJSON(encoded []byte) error {
	if r == nil {
		return errors.New("run limits: nil JSON target")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var wire runLimitsJSON
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("run limits: decode: %w", err)
	}
	if err := rejectRunLimitsTrailingJSON(decoder); err != nil {
		return err
	}
	var parsed RunLimits
	switch wire.Kind {
	case unlimitedRunLimits:
		if wire.MaxTotalTokens != nil || wire.MaxSteps != nil || wire.MaxBudgetUSD != nil {
			return errors.New("run limits: unlimited policy carries limits")
		}
		parsed = UnlimitedRunLimits()
	case limitedRunLimits:
		var err error
		parsed, err = NewRunLimits(RunLimitValues{
			MaxTotalTokens: wire.MaxTotalTokens,
			MaxSteps:       wire.MaxSteps,
			MaxBudgetUSD:   wire.MaxBudgetUSD,
		})
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("run limits: type %q is invalid", wire.Kind)
	}
	*r = parsed
	return nil
}

func rejectRunLimitsTrailingJSON(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("run limits: trailing JSON: %w", err)
	}
	return errors.New("run limits: trailing JSON value")
}
