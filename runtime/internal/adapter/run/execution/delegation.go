package execution

import (
	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	"github.com/Tangerg/scope/agent/workflow"
	corechat "github.com/Tangerg/scope/core/chat"
)

const defaultDelegateDepth = 4

func compileDelegatedInteraction(
	compiler *scoperun.Compiler,
	name, revision string,
	inner scoperun.Program[interaction.Input, interaction.Output],
	instructions []corechat.Message,
	options corechat.Options,
) (scoperun.Program[delegateInput, interaction.Output], error) {
	return workflow.Adapt(compiler, workflow.AdaptConfig{
		Name: name, Description: delegateDescription, Revision: revision,
		Configuration: struct {
			Instructions []corechat.Message
			Options      corechat.Options
		}{cloneChatMessages(instructions), options.Clone()},
	}, inner, func(task delegateInput) (interaction.Input, error) {
		messages := cloneChatMessages(instructions)
		messages = append(messages, corechat.NewUserMessage(corechat.NewTextPart(task.Instructions)))
		return interaction.Input{Messages: messages, Options: options.Clone()}, nil
	}, func(output interaction.Output) (interaction.Output, error) {
		return output, nil
	})
}
