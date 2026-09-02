package agentexec

import (
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	corechat "github.com/Tangerg/scope/core/chat"
)

var errInteractionAllowanceDenied = errors.New("agentexec: Run allowance denies another model call")

type interactionAllowanceStop uint8

const (
	interactionAllowanceOpen interactionAllowanceStop = iota
	interactionAllowanceStepsExhausted
	interactionAllowanceBudgetExhausted
	interactionAllowancePricingUnavailable
)

// interactionAllowance owns admission of the next model call. Limits are
// cumulative product policy, not provider request options: a completed call is
// first committed and metered, then this owner decides whether another call is
// allowed. That lets a final answer complete at the boundary without admitting
// a follow-up round that would exceed it.
type interactionAllowance struct {
	limits run.Limits

	mu   sync.Mutex
	stop interactionAllowanceStop
}

func newInteractionAllowance(
	limits run.Limits,
	selection modelref.Selection,
	pricing accounting.Pricing,
) (*interactionAllowance, error) {
	if err := limits.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", runs.ErrInvalidRunLimit, err)
	}
	if _, limited := limits.MaxBudgetUSD(); limited {
		if pricing == nil {
			return nil, fmt.Errorf("%w: a cost limit requires model pricing", runs.ErrInvalidRunLimit)
		}
		probe := pricing(selection.Provider(), selection.Model(), new(corechat.Usage))
		if err := probe.Validate(); err != nil {
			return nil, fmt.Errorf("%w: model pricing probe: %v", runs.ErrInvalidRunLimit, err)
		}
		if _, available := probe.USD(); !available {
			return nil, fmt.Errorf(
				"%w: model %q/%q has no pricing for a cost-limited Run",
				runs.ErrInvalidRunLimit,
				selection.Provider(),
				selection.Model(),
			)
		}
	}
	return &interactionAllowance{limits: limits}, nil
}

func (a *interactionAllowance) admit(snapshot accounting.Snapshot) error {
	if a == nil {
		return errors.New("agentexec: Run allowance is unavailable")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stop != interactionAllowanceOpen {
		return errInteractionAllowanceDenied
	}
	stop, err := allowanceStop(a.limits, snapshot)
	if err != nil {
		return err
	}
	if stop == interactionAllowanceOpen {
		return nil
	}
	a.stop = stop
	return errInteractionAllowanceDenied
}

func allowanceStop(limits run.Limits, snapshot accounting.Snapshot) (interactionAllowanceStop, error) {
	if len(snapshot.Models) == 0 {
		return interactionAllowanceOpen, nil
	}
	total, err := snapshot.Total()
	if err != nil {
		return interactionAllowanceOpen, fmt.Errorf("agentexec: evaluate Run allowance: %w", err)
	}
	if maximum, limited := limits.MaxSteps(); limited && total.Calls >= maximum {
		return interactionAllowanceStepsExhausted, nil
	}
	stop, err := tokenAllowanceStop(limits, total)
	if err != nil || stop != interactionAllowanceOpen {
		return stop, err
	}
	return costAllowanceStop(limits, total), nil
}

func tokenAllowanceStop(limits run.Limits, total accounting.ModelUsage) (interactionAllowanceStop, error) {
	if maximum, limited := limits.MaxTotalTokens(); limited {
		used, err := total.Total()
		if err != nil {
			return interactionAllowanceOpen, fmt.Errorf("agentexec: cumulative token usage: %w", err)
		}
		if used >= maximum {
			return interactionAllowanceBudgetExhausted, nil
		}
	}
	return interactionAllowanceOpen, nil
}

func costAllowanceStop(limits run.Limits, total accounting.ModelUsage) interactionAllowanceStop {
	if maximum, limited := limits.MaxBudgetUSD(); limited {
		cost, available := total.Cost.USD()
		switch {
		case !available:
			return interactionAllowancePricingUnavailable
		case cost >= maximum:
			return interactionAllowanceBudgetExhausted
		}
	}
	return interactionAllowanceOpen
}

func (a *interactionAllowance) terminal() interactionAllowanceStop {
	if a == nil {
		return interactionAllowanceOpen
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stop
}
