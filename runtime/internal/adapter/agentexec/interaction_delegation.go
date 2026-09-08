package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

// delegationPolicy bounds managed children independently of product token and
// model-call limits. Each remaining recursion level reserves one process budget
// so delegation never renews or duplicates framework work allocation.
type delegationPolicy struct {
	treeLimits    agent.TreeLimits
	processBudget agent.Budget
}

func newDelegationPolicy() (delegationPolicy, error) {
	budget, err := agent.NewBudget(agent.BudgetConfig{Steps: 256, Effects: 256, Signals: 2048})
	if err != nil {
		return delegationPolicy{}, fmt.Errorf("agentexec: Interaction delegation budget: %w", err)
	}
	return delegationPolicy{
		treeLimits:    agent.TreeLimits{MaxDepth: 4, MaxChildren: 16, MaxActiveChildren: 4, MaxTreeProcesses: 64},
		processBudget: budget,
	}, nil
}

func delegateSubtreeBudget(base agent.Budget, processLevels uint32) (agent.Budget, error) {
	if !base.Valid() || processLevels == 0 {
		return agent.Budget{}, errors.New("agentexec: delegated subtree budget requires a positive base and depth")
	}
	scale := uint64(processLevels)
	if base.Steps > math.MaxUint64/scale || base.Effects > math.MaxUint64/scale ||
		base.Signals > math.MaxUint64/scale {
		return agent.Budget{}, errors.New("agentexec: delegated subtree budget overflows")
	}
	budget, err := agent.NewBudget(agent.BudgetConfig{
		Steps: base.Steps * scale, Effects: base.Effects * scale, Signals: base.Signals * scale,
	})
	if err != nil {
		return agent.Budget{}, fmt.Errorf("agentexec: delegated subtree budget: %w", err)
	}
	return budget, nil
}

type delegatedTaskOutput struct {
	Reply string `json:"reply" jsonschema:"minLength=1"`
}

// delegatedInteractionDefinition is an ACL Definition: models execute an
// ordinary Interaction, while the managed Delegate boundary exposes the
// stable delegate_task input/output contract instead of Interaction's Host chat
// envelope. Snapshot interpretation remains exclusively Interaction-owned.
type delegatedInteractionDefinition struct {
	descriptor   agent.Descriptor
	inner        *interaction.Definition
	instructions []corechat.Message
}

func newDelegatedInteractionDefinition(
	name string,
	inner *interaction.Definition,
	instructions []corechat.Message,
) (*delegatedInteractionDefinition, error) {
	if inner == nil {
		return nil, errors.New("agentexec: delegated Interaction definition is nil")
	}
	inputSchema, err := runtimeContractSchema[delegateInput]()
	if err != nil {
		return nil, fmt.Errorf("agentexec: delegated task input schema: %w", err)
	}
	outputSchema, err := runtimeContractSchema[delegatedTaskOutput]()
	if err != nil {
		return nil, fmt.Errorf("agentexec: delegated task output schema: %w", err)
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: name, Description: delegateDescription,
		InputSchema: inputSchema, OutputSchema: outputSchema,
	})
	if err != nil {
		return nil, fmt.Errorf("agentexec: delegated Interaction descriptor: %w", err)
	}
	return &delegatedInteractionDefinition{
		descriptor: descriptor, inner: inner,
		instructions: cloneChatMessages(instructions),
	}, nil
}

// runtimeContractSchema derives the delegated contract through the Agent
// Framework's schema owner. Both sides use Core's canonical JSON Schema
// implementation, so the wire vocabulary has no adapter-local representation.
func runtimeContractSchema[T any]() (agent.Schema, error) {
	return agent.SchemaFor[T]()
}

func (d *delegatedInteractionDefinition) Descriptor() agent.Descriptor {
	if d == nil {
		return agent.Descriptor{}
	}
	return d.descriptor
}

func (d *delegatedInteractionDefinition) Start(input agent.Input) (agent.Execution, error) {
	if d == nil || d.inner == nil || !d.descriptor.Valid() {
		return nil, errors.New("agentexec: delegated Interaction definition is invalid")
	}
	task, err := input.Decode[delegateInput]()
	if err != nil {
		return nil, fmt.Errorf("agentexec: decode delegated task: %w", err)
	}
	if validateErr := task.Validate(); validateErr != nil {
		return nil, fmt.Errorf("agentexec: invalid delegated task: %w", validateErr)
	}
	messages := cloneChatMessages(d.instructions)
	messages = append(messages, corechat.NewUserMessage(corechat.NewTextPart(task.Instructions)))
	adapted, err := agent.EncodeInput(interaction.Input{Messages: messages})
	if err != nil {
		return nil, fmt.Errorf("agentexec: encode delegated Interaction input: %w", err)
	}
	execution, err := d.inner.Start(adapted)
	if err != nil {
		return nil, err
	}
	return &delegatedInteractionExecution{inner: execution}, nil
}

func (d *delegatedInteractionDefinition) Restore(
	state agent.ExecutionState,
) (agent.Execution, error) {
	if d == nil || d.inner == nil || !d.descriptor.Valid() {
		return nil, errors.New("agentexec: delegated Interaction definition is invalid")
	}
	execution, err := d.inner.Restore(state)
	if err != nil {
		return nil, err
	}
	return &delegatedInteractionExecution{inner: execution}, nil
}

type delegatedInteractionExecution struct{ inner agent.Execution }

func (d *delegatedInteractionExecution) Step(
	ctx context.Context,
	signals []agent.Signal,
) (agent.Transition, error) {
	if d == nil || d.inner == nil {
		return agent.Transition{}, errors.New("agentexec: delegated Interaction execution is invalid")
	}
	transition, err := d.inner.Step(ctx, signals)
	if err != nil || transition.Kind() != agent.TransitionKindComplete {
		return transition, err
	}
	erased, _ := transition.Output()
	output, err := erased.Decode[interaction.Output]()
	if err != nil {
		return agent.Transition{}, fmt.Errorf("agentexec: decode delegated Interaction output: %w", err)
	}
	reply, err := delegatedInteractionReply(output)
	if err != nil {
		return agent.Transition{}, err
	}
	adapted, err := agent.EncodeOutput(delegatedTaskOutput{Reply: reply})
	if err != nil {
		return agent.Transition{}, fmt.Errorf("agentexec: encode delegated task output: %w", err)
	}
	return agent.Complete(transition.ConsumedSignals(), adapted)
}

func (d *delegatedInteractionExecution) Snapshot() (agent.ExecutionState, error) {
	if d == nil || d.inner == nil {
		return agent.ExecutionState{}, errors.New("agentexec: delegated Interaction execution is invalid")
	}
	return d.inner.Snapshot()
}

func delegatedInteractionReply(output interaction.Output) (string, error) {
	if err := output.Validate(); err != nil {
		return "", err
	}
	switch output.Source {
	case interaction.CompletionSourceModelResponse:
		reply := delegatedMessageReply(*output.ModelResponse.Output.Message)
		if reply == "" {
			return "", errors.New("agentexec: delegated Interaction completed without a textual answer")
		}
		return reply, nil
	case interaction.CompletionSourceDirectToolResults:
		encoded, err := json.Marshal(output.DirectToolResults)
		if err != nil {
			return "", fmt.Errorf("agentexec: encode delegated direct Tool results: %w", err)
		}
		return string(encoded), nil
	default:
		return "", fmt.Errorf("agentexec: unsupported delegated Interaction completion source %q", output.Source)
	}
}

func delegatedMessageReply(message corechat.Message) string {
	var reply strings.Builder
	for _, part := range message.Parts {
		switch part.Kind {
		case corechat.PartText, corechat.PartRefusal:
			reply.WriteString(part.Text)
		}
	}
	return reply.String()
}
