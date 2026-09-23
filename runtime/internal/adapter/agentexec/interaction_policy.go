package agentexec

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/optional"
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
	deltaBuffer, err := optional.Positive(config.DeltaBufferCapacity, defaultInteractionDeltaBuffer, "delta buffer capacity")
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction policy: %w", err)
	}
	toolConcurrency, err := optional.Positive(config.MaxConcurrentToolCalls, defaultInteractionConcurrentToolCalls, "maximum concurrent Tool calls")
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
	unknownPoll, err := optional.Positive(config.UnknownEffectPollInterval, defaultUnknownEffectPollInterval, "unknown-Effect poll interval")
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction policy: %w", err)
	}
	statePoll, err := optional.Positive(config.StatePollInterval, defaultInteractionStatePoll, "state poll interval")
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
