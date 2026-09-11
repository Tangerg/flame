package agentexec

import (
	"fmt"

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
	toolBudget, err := effectiveToolBudget(config)
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction Tool policy: %w", err)
	}
	toolBatchCeiling, err := positiveOrDefault(
		config.ToolBatchCeiling, defaultInteractionToolBatchCeiling, "Tool batch ceiling",
	)
	if err != nil {
		return interactionExecutionPolicy{}, fmt.Errorf("agentexec: Interaction Tool policy: %w", err)
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
		toolBudget:                toolBudget,
		toolBatchCeiling:          toolBatchCeiling,
		toolResultOffload:         toolResultOffload,
	}, nil
}

// effectiveToolBudget resolves the allocation each ordinary Tool child receives.
func effectiveToolBudget(config InteractionExecutorConfig) (agent.Budget, error) {
	steps, err := positiveOrDefault(config.ToolSteps, defaultInteractionToolSteps, "Tool steps")
	if err != nil {
		return agent.Budget{}, err
	}
	effects, err := positiveOrDefault(config.ToolEffects, defaultInteractionToolEffects, "Tool effects")
	if err != nil {
		return agent.Budget{}, err
	}
	signals, err := positiveOrDefault(config.ToolSignals, defaultInteractionToolSignals, "Tool signals")
	if err != nil {
		return agent.Budget{}, err
	}
	budget := agent.Budget{Steps: steps, Effects: effects, Signals: signals}
	if !budget.Valid() {
		return agent.Budget{}, fmt.Errorf("Tool budget is invalid")
	}
	return budget, nil
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
