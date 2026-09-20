package agentexec

import (
	"errors"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
)

func checkpointToolResultIDs(state interactionCheckpointState) []toolresult.ID {
	var ids []toolresult.ID
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
		return interactionCheckpointState{}, errors.New("agentexec: checkpoint result ownership differs from its continuation")
	}
	return state, nil
}
