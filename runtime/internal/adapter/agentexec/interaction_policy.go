package agentexec

import (
	"fmt"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"time"
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
	defaultMaxModelCalls      uint32
	deltaBufferCapacity       int
	maxConcurrentToolCalls    int
	unknownEffectPollInterval time.Duration
	statePollInterval         time.Duration
	delegation                effectiveInteractionDelegation
	toolResultOffload         toolResultOffloadPolicy
}

func newInteractionExecutionPolicy(config InteractionExecutorConfig) (interactionExecutionPolicy, error) {
	maxModelCalls, err := positiveOrDefault(config.DefaultMaxModelCalls, defaultInteractionModelCalls, "default maximum model calls")
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction policy: %w", err)
	}
	deltaBuffer, err := positiveOrDefault(config.DeltaBufferCapacity, defaultInteractionDeltaBuffer, "delta buffer capacity")
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction policy: %w", err)
	}
	toolConcurrency, err := positiveOrDefault(config.MaxConcurrentToolCalls, defaultInteractionConcurrentToolCalls, "maximum concurrent Tool calls")
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction policy: %w", err)
	}
	unknownPoll, err := positiveOrDefault(config.UnknownEffectPollInterval, defaultUnknownEffectPollInterval, "unknown-Effect poll interval")
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction policy: %w", err)
	}
	statePoll, err := positiveOrDefault(config.StatePollInterval, defaultInteractionStatePoll, "state poll interval")
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction policy: %w", err)
	}
	delegation, err := effectiveDelegation(config.Delegation)
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction delegation policy: %w", err)
	}
	toolResultOffload, err := newToolResultOffloadPolicy(config.ToolResultOffload)
	if err != nil {
		return interactionExecutionPolicy{}, err
	}
	if toolResultOffload.enabled && dependency.Missing(config.ToolResultStore) {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: enabled Tool-result offload requires a store")
	}
	return interactionExecutionPolicy{
		defaultMaxModelCalls:      maxModelCalls,
		deltaBufferCapacity:       deltaBuffer,
		maxConcurrentToolCalls:    toolConcurrency,
		unknownEffectPollInterval: unknownPoll,
		statePollInterval:         statePoll,
		delegation:                delegation,
		toolResultOffload:         toolResultOffload,
	}, nil
}

// positiveNumber is every limit shape Interaction policy and delegation accept:
// counts, budgets and intervals, signed or not.
type positiveNumber interface {
	~int | ~int64 | ~uint32 | ~uint64
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
