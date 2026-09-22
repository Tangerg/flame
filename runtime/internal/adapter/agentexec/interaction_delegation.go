package agentexec

import (
	"context"
	"errors"
	"fmt"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

const defaultDelegateDepth = 4

// delegatedInteractionDefinition adapts Runtime's task input to an Interaction.
// Execution, recovery state, and the complete output remain Scope-owned.
type delegatedInteractionDefinition struct {
	descriptor   agent.Descriptor
	inner        *interaction.Definition
	instructions []corechat.Message
	options      corechat.Options
}

func newDelegatedInteractionDefinition(
	name string,
	inner *interaction.Definition,
	instructions []corechat.Message,
	options corechat.Options,
) (*delegatedInteractionDefinition, error) {
	if inner == nil {
		return nil, errors.New("agentexec: delegated Interaction definition is nil")
	}
	inputSchema, err := agent.SchemaFor[delegateInput]()
	if err != nil {
		return nil, fmt.Errorf("agentexec: delegated task input schema: %w", err)
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: name, Description: delegateDescription,
		InputSchema: inputSchema, OutputSchema: inner.Descriptor().OutputSchema(), SignalSchema: inner.Descriptor().SignalSchema(),
	})
	if err != nil {
		return nil, fmt.Errorf("agentexec: delegated Interaction descriptor: %w", err)
	}
	return &delegatedInteractionDefinition{
		descriptor: descriptor, inner: inner,
		instructions: cloneChatMessages(instructions), options: options.Clone(),
	}, nil
}

func (d *delegatedInteractionDefinition) Descriptor() agent.Descriptor {
	return d.descriptor
}

func (d *delegatedInteractionDefinition) Start(input agent.Payload) (agent.Execution, error) {
	if err := d.descriptor.ValidateInput(input); err != nil {
		return nil, err
	}
	task, err := input.Decode[delegateInput]()
	if err != nil {
		return nil, fmt.Errorf("agentexec: decode delegated task: %w", err)
	}
	messages := cloneChatMessages(d.instructions)
	messages = append(messages, corechat.NewUserMessage(corechat.NewTextPart(task.Instructions)))
	adapted, err := agent.EncodePayload(interaction.Input{Messages: messages, Options: d.options.Clone()})
	if err != nil {
		return nil, fmt.Errorf("agentexec: encode delegated Interaction input: %w", err)
	}
	return d.inner.Start(adapted)
}

func (d *delegatedInteractionDefinition) Restore(
	ctx context.Context,
	state agent.ExecutionState,
) (agent.Execution, error) {
	return d.inner.Restore(ctx, state)
}
