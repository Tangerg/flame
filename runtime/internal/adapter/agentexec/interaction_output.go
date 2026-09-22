package agentexec

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
)

func completedAssistantMessage(result agent.Result) (*runs.AssistantMessageCompleted, error) {
	if result.Status() != agent.StatusCompleted {
		return nil, nil
	}
	payload, present := result.Output()
	if !present {
		return nil, errors.New("agentexec: completed Interaction has no output")
	}
	output, err := payload.Decode[interaction.Output]()
	if err != nil {
		return nil, fmt.Errorf("agentexec: decode Interaction output: %w", err)
	}
	if err := output.Validate(); err != nil {
		return nil, err
	}
	if output.Source == interaction.CompletionSourceDirectToolResults {
		// TreeCommitter already projects the exact Tool results as Tool Items.
		return nil, nil
	}
	completion, err := runs.NewAssistantMessageCompleted(*output.ModelResponse.Output.Message)
	if err != nil {
		return nil, err
	}
	return &completion, nil
}
