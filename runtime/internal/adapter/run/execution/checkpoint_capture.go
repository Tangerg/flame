package execution

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
)

func (i *interactionSession) executorCheckpoint(
	tree scoperun.Log,
) (runs.ExecutorCheckpoint, error) {
	payload, err := i.captureCheckpointPayload(tree)
	if err != nil {
		return runs.ExecutorCheckpoint{}, err
	}
	bindings, resources := i.checkpointPluginReferences()
	checkpoint := runs.ExecutorCheckpoint{
		Bindings:     bindings,
		Resources:    resources,
		RootMemberID: tree.Root.String(), SessionID: i.scope.SessionID,
		Payload: payload, BuildID: i.buildID.String(),
	}
	if err := checkpoint.Validate(); err != nil {
		return runs.ExecutorCheckpoint{}, err
	}
	return checkpoint, nil
}

func (i *interactionSession) captureCheckpointPayload(
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
	if instructions == nil {
		instructions = []corechat.Message{}
	}
	wire := interactionCheckpointPayloadWire{
		ToolProfile:      i.start.ToolProfile,
		ToolProposals:    new(contributionBindingsWire(i.toolProposals)),
		ModelObservers:   new(contributionBindingsWire(i.modelObservers)),
		ModelContexts:    new(contributionBindingsWire(i.modelContexts)),
		ModelInputs:      new(contributionBindingsWire(i.modelInputs)),
		MCPManifests:     &manifests,
		SkillDiscoveries: i.skillDiscoveryCommands(),
		ToolCalls:        metadata,
		Options:          new(executionOptions(i.start.ModelSelection, i.start.Options)),
		Handoff:          handoff,
		Instructions:     &instructions,
	}
	wire.ModelProvider, err = encodeModelProvider(provider)
	if err != nil {
		return nil, err
	}
	wire.Contexts, err = encodeInteractionModelContexts(i.material.checkpointCalibrations(), handoff.Processes)
	if err != nil {
		return nil, fmt.Errorf("execution: encode Interaction model contexts: %w", err)
	}
	wire.PendingSteers, err = encodeInteractionPendingSteers(pendingSteers)
	if err != nil {
		return nil, fmt.Errorf("execution: encode pending Interaction steers: %w", err)
	}
	wire.PendingResumeInputs, err = encodeInteractionResumeInputs(pendingResumeInputs)
	if err != nil {
		return nil, fmt.Errorf("execution: encode pending Resume inputs: %w", err)
	}
	return wire.encode()
}
