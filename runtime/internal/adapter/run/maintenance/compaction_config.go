package maintenance

import (
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// compactionDefaults govern the auto-compact trigger. Tunable via
// [CompactionPolicyValues]. Token footprint owns the request-fit decision. A
// separate message-count threshold may initiate proactive maintenance, but it
// can never make an otherwise valid request fail its capacity check.
const (
	percentageScale = 100

	defaultCompactMaxMessages = 24 // proactive maintenance trigger threshold
	defaultCompactKeepRecent  = 6  // raw messages to preserve verbatim

	// defaultCompactMaxTokens is the estimated-token-footprint trigger used
	// ONLY when the model's real context window is unknown (catalog miss). When
	// the window IS known the trigger is window-relative instead — see
	// [CompactionPolicyValues.FallbackTokenLimits] / [windowTriggerPct], capped by
	// the provider's hard input envelope when one is known.
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
// A sweep triggers when EITHER bound is breached: MaxMessages (raw
// message count) or MaxTokens (estimated token footprint). On a sweep the
// oldest (len - KeepRecent) messages are replaced by a single system
// message carrying an LLM-generated summary.
type CompactionPolicyValues struct {
	MaxMessages *int // nil selects defaultCompactMaxMessages
	MaxTokens   *int // optional explicit token-footprint trigger; capped by the provider's hard input limit
	KeepRecent  *int // nil selects defaultCompactKeepRecent
	// FallbackTokenLimits are the default model's complete context envelope,
	// used only when the selected model is absent from the catalog.
	FallbackTokenLimits modelref.TokenLimits
}

// compactionPolicy is the validated, immutable policy consumed by Compactor.
type compactionPolicy struct {
	messageTrigger    messageCountTrigger
	maxTokens         int
	maxTokensExplicit bool
	keepRecent        int
	fallbackLimits    modelref.TokenLimits
}

func newCompactionPolicy(values CompactionPolicyValues) (compactionPolicy, error) {
	messageTrigger := newMessageCountTrigger(defaultCompactMaxMessages)
	if values.MaxMessages != nil {
		if *values.MaxMessages <= 0 {
			return compactionPolicy{}, fmt.Errorf("compaction policy: maximum messages must be positive")
		}
		messageTrigger = newMessageCountTrigger(*values.MaxMessages)
	}
	keepRecent, err := positiveIntOrDefault(values.KeepRecent, defaultCompactKeepRecent, "recent messages")
	if err != nil {
		return compactionPolicy{}, fmt.Errorf("compaction policy: %w", err)
	}
	if messageTrigger.enabled() && keepRecent >= messageTrigger.limit() {
		return compactionPolicy{}, fmt.Errorf("compaction policy: recent messages %d must be less than maximum messages %d", keepRecent, messageTrigger.limit())
	}
	if err := values.FallbackTokenLimits.Validate(); err != nil {
		return compactionPolicy{}, fmt.Errorf("compaction policy: fallback token limits: %w", err)
	}

	policy := compactionPolicy{
		messageTrigger: messageTrigger,
		keepRecent:     keepRecent,
		fallbackLimits: values.FallbackTokenLimits,
	}
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
// model's window, the default model fallback, or the coarse fixed fallback.
// A provider's prompt envelope always remains the hard upper bound.
func (p compactionPolicy) tokenTrigger(limits modelref.TokenLimits, options chat.Options) (int, error) {
	effectiveLimits := limits
	if effectiveLimits.Unknown() {
		effectiveLimits = p.fallbackLimits
	}
	reservation := modelref.OutputReservation{}
	if options.MaxOutputTokens != nil {
		var err error
		reservation, err = modelref.NewOutputReservation(*options.MaxOutputTokens)
		if err != nil {
			return 0, err
		}
	}
	inputLimit, inputLimitKnown, err := effectiveLimits.InputCeiling(reservation)
	if err != nil {
		return 0, err
	}
	contextWindow, contextWindowKnown := effectiveLimits.ContextWindow()

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
