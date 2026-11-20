package execution

import (
	"maps"
	"slices"
	"strings"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	scoperun "github.com/Tangerg/scope/agent/run"
)

func (i *interactionSession) interactionCheckpointPayload(
	tree scoperun.Log,
) ([]byte, error) {
	i.state.mu.Lock()
	pendingSteers := maps.Clone(i.state.pendingSteers)
	pendingResumeInputs := slices.Clone(i.state.pendingResumeInputs)
	metadata := make([]toolCallCaptureWire, 0, len(i.state.toolMetadata))
	for _, value := range i.state.toolMetadata {
		if value.Receipt != nil || value.presentation == nil {
			metadata = append(metadata, value.wire())
		}
	}
	slices.SortFunc(metadata, func(left, right toolCallCaptureWire) int {
		return strings.Compare(left.CallID, right.CallID)
	})
	i.state.mu.Unlock()

	instructions, err := interactionInstructionContext(i.start.WorkingContext)
	if err != nil {
		return nil, err
	}
	manifests, err := i.checkpointMCPManifests()
	if err != nil {
		return nil, err
	}
	handoff, err := i.encodeInteractionHandoff(tree)
	if err != nil {
		return nil, err
	}
	var provider *modeladapter.ProviderCapture
	if capture, found := i.selectedChat.Capture(); found {
		provider = &capture
	}
	return encodeInteractionCheckpointPayload(
		tree,
		i.material.checkpointCalibrations(),
		instructions,
		pendingSteers,
		pendingResumeInputs,
		metadata,
		executionOptions(i.start.ModelSelection, i.start.Options),
		manifests,
		i.skillDiscoveryCommands(),
		handoff,
		i.modelObservers,
		i.modelContexts,
		i.modelInputs,
		i.toolProposals,
		provider,
	)
}
