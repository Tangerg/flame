package run

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	modelchoice "github.com/Tangerg/flame/runtime/model"
	"github.com/Tangerg/scope/core/chat"
)

// Metrics is the cumulative consumption of one Run across all of its Segments.
// Usage is absent until a provider reports it. ActiveDuration excludes time
// spent waiting for a person.
type Metrics struct {
	usage          *accounting.Usage
	steps          int
	activeDuration time.Duration
}

// NewMetrics constructs a validated cumulative metrics value.
func NewMetrics(usage *accounting.Usage, steps int, activeDuration time.Duration) (Metrics, error) {
	metrics := Metrics{steps: steps, activeDuration: activeDuration}
	if usage != nil {
		cloned := usage.Clone()
		metrics.usage = &cloned
	}
	if err := metrics.Validate(); err != nil {
		return Metrics{}, err
	}
	return metrics, nil
}

// Validate reports whether the cumulative values are internally consistent.
func (m Metrics) Validate() error {
	if m.steps < 0 || m.activeDuration < 0 {
		return errors.New("run: metrics must not be negative")
	}
	if m.usage != nil {
		if err := m.usage.Validate(); err != nil {
			return fmt.Errorf("run: metrics: %w", err)
		}
	}
	return nil
}

// ValidateAdvanceFrom proves that metrics is a monotonic continuation of the
// previous committed value.
func (m Metrics) ValidateAdvanceFrom(previous Metrics) error {
	if err := previous.Validate(); err != nil {
		return fmt.Errorf("run: previous metrics: %w", err)
	}
	if err := m.Validate(); err != nil {
		return err
	}
	if m.steps < previous.steps || m.activeDuration < previous.activeDuration {
		return errors.New("run: cumulative metrics regressed")
	}
	switch {
	case previous.usage != nil && m.usage == nil:
		return errors.New("run: cumulative usage disappeared")
	case previous.usage != nil:
		if err := m.usage.ValidateAdvanceFrom(*previous.usage); err != nil {
			return fmt.Errorf("run: usage: %w", err)
		}
	}
	return nil
}

// Equal reports semantic equality.
func (m Metrics) Equal(other Metrics) bool {
	if m.steps != other.steps || m.activeDuration != other.activeDuration {
		return false
	}
	if m.usage == nil || other.usage == nil {
		return m.usage == nil && other.usage == nil
	}
	return m.usage.Equal(*other.usage)
}

// Usage returns an ownership-isolated cumulative usage value.
func (m Metrics) Usage() (accounting.Usage, bool) {
	if m.usage == nil {
		return accounting.Usage{}, false
	}
	return m.usage.Clone(), true
}

// Cost answers what this Run has spent. An unreported usage is a Run that has
// not accounted yet, which is a zero cost rather than a failure — the same
// answer every caller needs, asked of the value that holds it.
func (m Metrics) Cost() (accounting.Cost, error) {
	if m.usage == nil {
		return accounting.Cost{}, nil
	}
	return accounting.CostFromOptional(m.usage.Total.CostUSD)
}

// Steps returns the cumulative model-call count.
func (m Metrics) Steps() int { return m.steps }

// ActiveDuration returns the cumulative time spent executing.
func (m Metrics) ActiveDuration() time.Duration { return m.activeDuration }

// AccrueModelCall adds one not-yet-committed provider call. The journal
// transition and Run replacement commit together; a restarted pass cannot add it twice.
func (m Metrics) AccrueModelCall(model string, reported *chat.Usage, costUSD *float64) (Metrics, error) {
	if err := m.Validate(); err != nil {
		return Metrics{}, err
	}
	if _, err := modelchoice.NewModelIdentity(model); err != nil {
		return Metrics{}, err
	}
	cost, err := accounting.CostFromOptional(costUSD)
	if err != nil {
		return Metrics{}, err
	}
	if m.steps == math.MaxInt {
		return Metrics{}, errors.New("run: model call count overflows")
	}
	if reported == nil {
		if costUSD != nil {
			return Metrics{}, errors.New("run: model price requires reported usage")
		}
		if m.usage != nil {
			usage := m.usage.Clone()
			usage.Total.CostUSD = nil
			if usage.ByModel != nil {
				value := usage.ByModel[model]
				value.CostUSD = nil
				usage.ByModel[model] = value
			}
			m.usage = &usage
		}
		return NewMetrics(m.usage, m.steps+1, m.activeDuration)
	}
	tokens, err := accounting.NewTokens(*reported)
	if err != nil {
		return Metrics{}, err
	}
	delta := accounting.Totals{Tokens: tokens, CostUSD: cost.OptionalUSD()}
	usage := accounting.Usage{Total: delta, ByModel: map[string]accounting.Totals{model: delta}}
	if m.usage == nil && m.steps > 0 {
		usage.Total.CostUSD = nil
		// Earlier unreported calls have no surviving model attribution here.
		// Preserve this reported breakdown, but its aggregate price is unknown.
		delta.CostUSD = nil
		usage.ByModel[model] = delta
	}
	if m.usage != nil {
		usage = m.usage.Clone()
		usage.Total, err = usage.Total.Add(delta)
		if err != nil {
			return Metrics{}, err
		}
		if usage.ByModel != nil {
			if current, exists := usage.ByModel[model]; exists {
				delta, err = current.Add(delta)
				if err != nil {
					return Metrics{}, err
				}
			} else if m.usage.Total.CostUSD == nil {
				// An unattributed earlier call may belong to this model. A later
				// known price cannot make that model's aggregate price exact.
				delta.CostUSD = nil
			}
			usage.ByModel[model] = delta
		}
	}
	return NewMetrics(&usage, m.steps+1, m.activeDuration)
}
