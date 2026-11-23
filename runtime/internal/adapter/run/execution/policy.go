package execution

import (
	"errors"
	"math"
	"time"

	"github.com/samber/lo"
)

const (
	defaultInteractionDeltaBuffer         = 256
	defaultInteractionConcurrentToolCalls = 8
)

type interactionExecutionPolicy struct {
	deltaBufferCapacity       int
	maxConcurrentToolCalls    int
	unknownEffectPollInterval time.Duration
	statePollInterval         time.Duration
	toolResultOffload         toolResultOffloadPolicy
}

func newInteractionExecutionPolicy(config InteractionExecutorConfig) (interactionExecutionPolicy, error) {
	deltaBuffer := lo.FromPtrOr(config.DeltaBufferCapacity, defaultInteractionDeltaBuffer)
	if deltaBuffer <= 0 {
		return interactionExecutionPolicy{}, errors.New("execution: interaction policy: delta buffer capacity must be positive")
	}
	toolConcurrency := lo.FromPtrOr(config.MaxConcurrentToolCalls, defaultInteractionConcurrentToolCalls)
	if toolConcurrency <= 0 {
		return interactionExecutionPolicy{}, errors.New("execution: interaction policy: maximum concurrent Tool calls must be positive")
	}
	// Tool concurrency reaches the Framework as agent.TreeLimits.MaxActiveChildren,
	// a uint32. Refusing the excess here is what lets every later conversion be a
	// conversion rather than a silent wrap to a smaller limit.
	if uint64(toolConcurrency) > math.MaxUint32 {
		return interactionExecutionPolicy{}, errors.New(
			"execution: Interaction policy: maximum concurrent Tool calls exceeds the Framework tree limit range",
		)
	}
	unknownPoll := lo.FromPtrOr(config.UnknownEffectPollInterval, defaultUnknownEffectPollInterval)
	if unknownPoll <= 0 {
		return interactionExecutionPolicy{}, errors.New("execution: interaction policy: unknown-Effect poll interval must be positive")
	}
	statePoll := lo.FromPtrOr(config.StatePollInterval, defaultInteractionStatePoll)
	if statePoll <= 0 {
		return interactionExecutionPolicy{}, errors.New("execution: interaction policy: state poll interval must be positive")
	}
	toolResultOffload, err := newToolResultOffloadPolicy(config.ToolResultOffloadBytes)
	if err != nil {
		return interactionExecutionPolicy{}, err
	}
	return interactionExecutionPolicy{
		deltaBufferCapacity:       deltaBuffer,
		maxConcurrentToolCalls:    toolConcurrency,
		unknownEffectPollInterval: unknownPoll,
		statePollInterval:         statePoll,
		toolResultOffload:         toolResultOffload,
	}, nil
}
