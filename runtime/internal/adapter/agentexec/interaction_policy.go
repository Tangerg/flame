package agentexec

import (
	"fmt"
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
	maxModelCalls, err := positiveUint32OrDefault(config.DefaultMaxModelCalls, defaultInteractionModelCalls, "default maximum model calls")
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction policy: %w", err)
	}
	deltaBuffer, err := positiveIntOrDefault(config.DeltaBufferCapacity, defaultInteractionDeltaBuffer, "delta buffer capacity")
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction policy: %w", err)
	}
	toolConcurrency, err := positiveIntOrDefault(config.MaxConcurrentToolCalls, defaultInteractionConcurrentToolCalls, "maximum concurrent Tool calls")
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction policy: %w", err)
	}
	unknownPoll, err := positiveDurationOrDefault(config.UnknownEffectPollInterval, defaultUnknownEffectPollInterval, "unknown-Effect poll interval")
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction policy: %w", err)
	}
	statePoll, err := positiveDurationOrDefault(config.StatePollInterval, defaultInteractionStatePoll, "state poll interval")
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
	if toolResultOffload.enabled && isNilInteractionCapability(config.ToolResultStore) {
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
