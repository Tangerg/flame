package execution

import (
	"errors"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func checkpointToolResultIDs(state interactionCheckpointState) []transcript.ToolResultID {
	var ids []transcript.ToolResultID
	for _, metadata := range state.toolMetadata {
		if metadata.Offload != nil {
			ids = append(ids, metadata.Offload.ID)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

func decodeExecutorCheckpoint(checkpoint runs.ExecutorCheckpoint) (interactionCheckpointState, error) {
	state, err := decodeInteractionCheckpointPayload(checkpoint.Payload)
	if err != nil {
		return interactionCheckpointState{}, err
	}
	if !slices.Equal(checkpoint.ToolResultIDs, checkpointToolResultIDs(state)) {
		return interactionCheckpointState{}, errors.New("execution: checkpoint result ownership differs from its continuation")
	}
	state.resources = slices.Clone(checkpoint.Resources)
	for _, activation := range checkpoint.Bindings {
		if _, err := plugins.DecodeActivation(activation); err != nil {
			return interactionCheckpointState{}, err
		}
		state.bindings = append(state.bindings, activation.Clone())
	}
	if state.modelProvider != nil {
		values, err := plugins.BindActivations([]plugins.ContributionBinding{state.modelProvider.Binding}, state.bindings)
		if err != nil {
			return interactionCheckpointState{}, err
		}
		state.modelProvider.Binding = values[0]
		if err := state.modelProvider.Validate(); err != nil {
			return interactionCheckpointState{}, err
		}
	}
	return state, nil
}
