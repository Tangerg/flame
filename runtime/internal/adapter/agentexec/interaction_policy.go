package agentexec

import (
	"errors"
	"fmt"
	"math"

	"github.com/Tangerg/flame/runtime/internal/dependency"
	agent "github.com/Tangerg/scope/agent"
	"time"
)

const (
	defaultInteractionDeltaBuffer         = 256
	defaultInteractionConcurrentToolCalls = 1
)

// A Tool call is one child Process: it executes once and then settles. Its
// budget is transferred permanently out of the parent's, so it is sized for a
// single call plus the input rounds that call may take, not for an Interaction.
// Signals are the bound on how many times a Tool may come back to the user,
// which is a limit Flame did not previously express at all.
const (
	defaultInteractionToolSteps   = 8
	defaultInteractionToolEffects = 8
	defaultInteractionToolSignals = 32

	// defaultInteractionToolBatchCeiling bounds how many Tool calls one model
	// response may turn into child Processes. It is the per-response half of the
	// lifetime Tool-call ceiling; the other half is the Interaction's own
	// model-call limit.
	defaultInteractionToolBatchCeiling = 16
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
	toolBudget                agent.Budget
	toolBatchCeiling          uint32
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
	delegation, err := interactionDelegation()
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction delegation policy: %w", err)
	}
	toolBudget := agent.Budget{
		Steps:   defaultInteractionToolSteps,
		Effects: defaultInteractionToolEffects,
		Signals: defaultInteractionToolSignals,
	}
	if !toolBudget.Valid() {
		return interactionExecutionPolicy{}, errors.New("agentexec: Interaction Tool budget is invalid")
	}
	toolResultOffload, err := newToolResultOffloadPolicy(config.ToolResultOffload)
	if err != nil {
		return interactionExecutionPolicy{}, err
	}
	if toolResultOffload.enabled && dependency.Missing(config.ToolResultStore) {
		return interactionExecutionPolicy{}, errors.New("agentexec: enabled Tool-result offload requires a store")
	}
	return interactionExecutionPolicy{
		defaultMaxModelCalls:      maxModelCalls,
		deltaBufferCapacity:       deltaBuffer,
		maxConcurrentToolCalls:    toolConcurrency,
		unknownEffectPollInterval: unknownPoll,
		statePollInterval:         statePoll,
		delegation:                delegation,
		toolBudget:                toolBudget,
		toolBatchCeiling:          defaultInteractionToolBatchCeiling,
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
