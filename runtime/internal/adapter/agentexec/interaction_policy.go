package agentexec

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/flame/runtime/internal/dependency"
)

const (
	defaultInteractionDeltaBuffer         = 256
	defaultInteractionConcurrentToolCalls = 1
)

// interactionExecutionPolicy is the validated, immutable execution policy
// shared by every Session and Deployment owned by one InteractionExecutor.
// Construction values may be absent, but consumers never receive sentinel
// zeroes or defer defaults to the Agent Framework.
type interactionExecutionPolicy struct {
	deltaBufferCapacity       int
	maxConcurrentToolCalls    int
	unknownEffectPollInterval time.Duration
	statePollInterval         time.Duration
	toolResultOffload         toolResultOffloadPolicy
}

func newInteractionExecutionPolicy(config InteractionExecutorConfig) (interactionExecutionPolicy, error) {
	deltaBuffer, err := positiveOrDefault(config.DeltaBufferCapacity, defaultInteractionDeltaBuffer, "delta buffer capacity")
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction policy: %w", err)
	}
	toolConcurrency, err := positiveOrDefault(config.MaxConcurrentToolCalls, defaultInteractionConcurrentToolCalls, "maximum concurrent Tool calls")
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction policy: %w", err)
	}
	// Tool concurrency reaches the Framework as agent.TreeLimits.MaxActiveChildren,
	// a uint32. Refusing the excess here is what lets every later conversion be a
	// conversion rather than a silent wrap to a smaller limit.
	if uint64(toolConcurrency) > math.MaxUint32 {
		return interactionExecutionPolicy{}, errors.New(
			"agentexec: Interaction policy: maximum concurrent Tool calls exceeds the Framework tree limit range",
		)
	}
	unknownPoll, err := positiveOrDefault(config.UnknownEffectPollInterval, defaultUnknownEffectPollInterval, "unknown-Effect poll interval")
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction policy: %w", err)
	}
	statePoll, err := positiveOrDefault(config.StatePollInterval, defaultInteractionStatePoll, "state poll interval")
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction policy: %w", err)
	}
	toolResultOffload, err := newToolResultOffloadPolicy(config.ToolResultOffload)
	if err != nil {
		return interactionExecutionPolicy{}, err
	}
	if toolResultOffload.enabled && dependency.Missing(config.ToolResultStore) {
		return interactionExecutionPolicy{}, errors.New("agentexec: enabled Tool-result offload requires a store")
	}
	return interactionExecutionPolicy{
		deltaBufferCapacity:       deltaBuffer,
		maxConcurrentToolCalls:    toolConcurrency,
		unknownEffectPollInterval: unknownPoll,
		statePollInterval:         statePoll,
		toolResultOffload:         toolResultOffload,
	}, nil
}

// positiveNumber covers finite capacities and polling intervals.
type positiveNumber interface {
	~int | ~int64
}

// positiveOrDefault admits an optional override, or the fallback when none was
// given. Both must be positive, because a zero limit is not a smaller limit —
// it is a policy that admits nothing.
func positiveOrDefault[T positiveNumber](value *T, fallback T, field string) (T, error) {
	var zero T
	if fallback <= zero {
		return zero, fmt.Errorf("%s default must be positive", field)
	}
	if value == nil {
		return fallback, nil
	}
	if *value <= zero {
		return zero, fmt.Errorf("%s must be positive", field)
	}
	return *value, nil
}
