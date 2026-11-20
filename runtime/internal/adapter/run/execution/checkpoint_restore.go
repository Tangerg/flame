package execution

import (
	"errors"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
)

func decodeExecutorCheckpoint(checkpoint runs.ExecutorCheckpoint) (interactionCheckpointState, error) {
	state, err := decodeInteractionCheckpointPayload(checkpoint.Payload)
	if err != nil {
		return interactionCheckpointState{}, err
	}
	if state.handoff.rootID.String() != checkpoint.RootMemberID {
		return interactionCheckpointState{}, errors.New("execution: checkpoint payload names a different root")
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

func decodeRunCheckpoint(checkpoint runs.ExecutorCheckpoint, sessionID identity.SessionID, runID identity.RunID) (interactionCheckpointState, error) {
	start := interactionStartID(sessionID, runID)
	if !start.Valid() || checkpoint.SessionID != sessionID || checkpoint.RootMemberID != start.Root().String() {
		return interactionCheckpointState{}, errors.New("execution: checkpoint belongs to a different Run")
	}
	return decodeExecutorCheckpoint(checkpoint)
}
