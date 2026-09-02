package maintenance

import (
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// compactionDefaults govern model-context reduction. Tunable via
// [CompactionPolicyValues]. The complete request's token footprint is the only
// compaction trigger; protocol message count is not a measure of context
// pressure.
const (
	percentageScale = 100

	// defaultCompactMaxTokens is the estimated-token-footprint trigger used
	// ONLY when the model's real context window is unknown (catalog miss). When
	// the window is known the trigger is window-relative instead, capped by the
	// provider's hard input envelope when one is known.
	defaultCompactMaxTokens = 100_000

	// windowTriggerPct is the share of the model's context window at which an
	// estimated footprint triggers compaction — leaving headroom for the
	// summary output + the next Run. A fixed number is wrong across the 32k…1M
	// window range; a relative trigger tracks the actual model's context
	// window rather than a fixed number that's wrong at either extreme.
	windowTriggerPct = 80
)

// CompactionPolicyValues is the construction boundary for the auto-compaction
// policy. Nil means "use the named product default"; a present numeric zero is
// invalid and never doubles as absence.
//
// A sweep triggers only when the complete model request reaches MaxTokens.
type CompactionPolicyValues struct {
	MaxTokens *int // optional explicit token-footprint trigger; capped by the provider's hard input limit
}

// compactionPolicy is the validated, immutable policy consumed by Compactor.
type compactionPolicy struct {
	maxTokens         int
	maxTokensExplicit bool
}

func newCompactionPolicy(values CompactionPolicyValues) (compactionPolicy, error) {
	policy := compactionPolicy{}
	if values.MaxTokens != nil {
		if *values.MaxTokens <= 0 {
			return compactionPolicy{}, fmt.Errorf("compaction policy: maximum tokens must be positive")
		}
		policy.maxTokens = *values.MaxTokens
		policy.maxTokensExplicit = true
	}
	return policy, nil
}

// tokenTrigger resolves the token-footprint compaction threshold for a Run.
// An explicit maximum wins; otherwise the threshold follows the selected
// model's window or the coarse fixed fallback for an unknown model.
// A provider's prompt envelope always remains the hard upper bound.
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
	if p.maxTokensExplicit {
		trigger = p.maxTokens
	} else if contextWindowKnown {
		window := tokenLimitInt(contextWindow)
		whole := window / percentageScale * windowTriggerPct
		fraction := window % percentageScale * windowTriggerPct / percentageScale
		trigger = max(1, saturatedAdd(whole, fraction))
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

func positiveIntOrDefault(value *int, fallback int, field string) (int, error) {
	if fallback <= 0 {
		return 0, fmt.Errorf("%s default must be positive", field)
	}
	if value == nil {
		return fallback, nil
	}
	if *value <= 0 {
		return 0, fmt.Errorf("%s must be positive", field)
	}
	return *value, nil
}

func positiveDurationOrDefault(value *time.Duration, fallback time.Duration, field string) (time.Duration, error) {
	if fallback <= 0 {
		return 0, fmt.Errorf("%s default must be positive", field)
	}
	if value == nil {
		return fallback, nil
	}
	if *value <= 0 {
		return 0, fmt.Errorf("%s must be positive", field)
	}
	return *value, nil
}
