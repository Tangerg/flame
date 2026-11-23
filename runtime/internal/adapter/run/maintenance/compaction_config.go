package maintenance

import (
	"errors"
	"math"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/core/chat"
)

const (
	percentageScale = 100

	defaultCompactMaxTokens = 100_000

	// Relative because a fixed number is wrong across the 32k…1M window range.
	windowTriggerPct = 80
)

type CompactionPolicyValues struct {
	MaxTokens *int
}

type compactionPolicy struct {
	maxTokens int
}

func newCompactionPolicy(values CompactionPolicyValues) (compactionPolicy, error) {
	policy := compactionPolicy{}
	if values.MaxTokens != nil {
		if *values.MaxTokens <= 0 {
			return compactionPolicy{}, errors.New("compaction policy: maximum tokens must be positive")
		}
		policy.maxTokens = *values.MaxTokens
	}
	return policy, nil
}

func (p compactionPolicy) tokenTrigger(limits modelref.TokenLimits, options chat.Options) (int, error) {
	reservation := modelref.OutputReservation{}
	if options.MaxOutputTokens != nil {
		var err error
		reservation, err = modelref.NewOutputReservation(*options.MaxOutputTokens)
		if err != nil {
			return 0, err
		}
	}
	inputLimit, inputLimitKnown, err := limits.InputCeiling(reservation)
	if err != nil {
		return 0, err
	}
	contextWindow, contextWindowKnown := limits.ContextWindow()

	trigger := defaultCompactMaxTokens
	if p.maxTokens > 0 {
		trigger = p.maxTokens
	} else if contextWindowKnown {
		window := tokenLimitInt(contextWindow)
		whole := window / percentageScale * windowTriggerPct
		fraction := window % percentageScale * windowTriggerPct / percentageScale
		trigger = math.MaxInt
		if whole <= math.MaxInt-fraction {
			trigger = max(1, whole+fraction)
		}
	}
	if inputLimitKnown {
		trigger = min(trigger, tokenLimitInt(inputLimit))
	}
	return trigger, nil
}

func tokenLimitInt(value int64) int {
	if value <= 0 {
		return 0
	}
	if uint64(value) > uint64(math.MaxInt) {
		return math.MaxInt
	}
	return int(value)
}
