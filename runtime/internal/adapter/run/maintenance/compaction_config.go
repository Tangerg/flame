package maintenance

import (
	"math"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// The complete request's token footprint is the only
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

// modelContextTokenTrigger follows the selected model's context window or the
// coarse fixed fallback for an unknown model.
// A provider's prompt envelope always remains the hard upper bound.
func modelContextTokenTrigger(limits modelref.TokenLimits, options chat.Options) (int, error) {
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
	if contextWindowKnown {
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
