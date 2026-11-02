package run

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/core/chat"
)

// Metrics is the cumulative consumption of one Run across all of its Segments.
// Usage is absent until a provider reports it. ActiveDuration excludes time
// spent waiting for a person.
type Metrics struct {
	usage            *accounting.Usage
	unreportedModels []string
	steps            int
	activeDuration   time.Duration
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

// Before the first usage report, keep the identities of dispatched calls
// without manufacturing a reported zero. The first real report absorbs
// those identities into its per-model cost breakdown.
func RestoreUnreportedMetrics(models []string, steps int, duration time.Duration) (Metrics, error) {
	m := Metrics{unreportedModels: slices.Clone(models), steps: steps, activeDuration: duration}
	if len(models) == 0 || len(models) > steps || !slices.IsSorted(models) {
		return Metrics{}, errors.New("run: invalid unreported model identities")
	}
	for i, model := range models {
		if _, err := modelchoice.NewModelIdentity(model); err != nil {
			return Metrics{}, err
		}
		if i > 0 && models[i-1] == model {
			return Metrics{}, errors.New("run: duplicate unreported model")
		}
	}
	return m, m.Validate()
}
func (m Metrics) UnreportedModels() []string { return slices.Clone(m.unreportedModels) }
func (m Metrics) WithActiveDuration(duration time.Duration) (Metrics, error) {
	m.activeDuration = duration
	return m, m.Validate()
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
	if len(previous.unreportedModels) > 0 {
		for _, model := range previous.unreportedModels {
			if m.usage == nil {
				if _, found := slices.BinarySearch(m.unreportedModels, model); !found {
					return errors.New("run: unreported model attribution disappeared")
				}
			} else if m.usage.ByModel != nil {
				part, found := m.usage.ByModel[model]
				if !found || part.Cost != (accounting.Cost{}) {
					return errors.New("run: unreported model became priced or disappeared")
				}
			}
		}
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

func (m Metrics) Equal(other Metrics) bool {
	if m.steps != other.steps || m.activeDuration != other.activeDuration || !slices.Equal(m.unreportedModels, other.unreportedModels) {
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

// Cost is exact zero for a Run that dispatched no model call and unpriced for
// one whose dispatched calls reported no usage: absence is never a zero price.
func (m Metrics) Cost() (accounting.Cost, error) {
	if m.usage == nil {
		if m.steps == 0 {
			return accounting.NewCost(0)
		}
		return accounting.Cost{}, nil
	}
	return m.usage.Total.Cost, nil
}

// Steps counts physically dispatched model calls, including unknown outcomes.
func (m Metrics) Steps() int { return m.steps }

// ActiveDuration returns the cumulative time spent executing.
func (m Metrics) ActiveDuration() time.Duration { return m.activeDuration }

// AccrueModelCall adds one not-yet-committed provider call. The journal
// transition and Run replacement commit together; a restarted pass cannot add it twice.
func (m Metrics) AccrueModelCall(model string, reported *chat.Usage, cost accounting.Cost) (Metrics, error) {
	if err := m.Validate(); err != nil {
		return Metrics{}, err
	}
	if _, err := modelchoice.NewModelIdentity(model); err != nil {
		return Metrics{}, err
	}
	if m.steps == math.MaxInt {
		return Metrics{}, errors.New("run: model call count overflows")
	}
	if reported == nil {
		if _, priced := cost.USD(); priced {
			return Metrics{}, errors.New("run: model price requires reported usage")
		}
		if m.usage != nil {
			usage, err := m.usage.Accrue(model, accounting.Totals{})
			if err != nil {
				return Metrics{}, err
			}
			m.usage = &usage
		} else if index, found := slices.BinarySearch(m.unreportedModels, model); !found {
			m.unreportedModels = slices.Insert(slices.Clone(m.unreportedModels), index, model)
		}
		m.steps++
		return m, nil
	}
	tokens, err := accounting.NewTokens(*reported)
	if err != nil {
		return Metrics{}, err
	}
	delta := accounting.Totals{Tokens: tokens, Cost: cost}
	var usage accounting.Usage
	switch {
	case m.usage != nil:
		usage, err = m.usage.Accrue(model, delta)
	case m.steps > 0 && len(m.unreportedModels) == 0:
		delta.Cost = accounting.Cost{}
		usage = accounting.Usage{Total: delta}
	default:
		parts := map[string]accounting.Totals{model: delta}
		for _, prior := range m.unreportedModels {
			part := parts[prior]
			part.Cost = accounting.Cost{}
			parts[prior] = part
		}
		usage, err = accounting.AttributedUsage(parts)
	}
	if err != nil {
		return Metrics{}, err
	}
	return NewMetrics(&usage, m.steps+1, m.activeDuration)
}
