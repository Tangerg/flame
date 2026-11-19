package execution

import (
	"encoding/base64"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"

	"encoding/json/jsontext"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"

	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
)

// installationDependencyWire binds installation releases into a Deployment's
// configuration identity.
type installationDependencyWire struct {
	InstallationID string `json:"installationId"`
	Digest         string `json:"digest"`
}

type interactionCheckpointPayloadWire struct {
	SkillDiscoveries    map[domaintool.Group]string    `json:"skill_discoveries,omitzero"`
	ModelProvider       jsontext.Value                 `json:"model_provider"`
	ToolProposals       *[]contributionBindingWire     `json:"tool_proposals"`
	ModelInputs         *[]contributionBindingWire     `json:"model_inputs"`
	ModelContexts       *[]contributionBindingWire     `json:"model_contexts"`
	ModelObservers      *[]contributionBindingWire     `json:"model_observers"`
	MCPManifests        *map[domaintool.Group][]string `json:"mcp_manifests"`
	Options             *corechat.Options              `json:"options"`
	ToolCalls           []toolCallCapture              `json:"tool_calls,omitempty"`
	Handoff             *interactionHandoffWire        `json:"handoff"`
	Instructions        *[]corechat.Message            `json:"instructions"`
	Contexts            []interactionModelContextWire  `json:"contexts,omitempty"`
	PendingSteers       []interactionPendingSteerWire  `json:"pending_steers,omitempty"`
	PendingResumeInputs []interactionResumeInputWire   `json:"pending_resume_inputs,omitempty"`
}

type interactionModelContextWire struct {
	ProcessID       string `json:"process_id"`
	ReportedTokens  int64  `json:"reported_tokens"`
	EstimatedTokens int    `json:"estimated_tokens"`
}

type interactionPendingSteerWire struct {
	Command *interactionSteerCommandWire  `json:"command,omitzero"`
	ItemID  string                        `json:"item_id"`
	Content []interactionContentBlockWire `json:"content"`
}

type interactionSteerCommandWire struct {
	Key         string `json:"key"`
	Fingerprint string `json:"fingerprint"`
	SessionID   string `json:"session_id"`
	RunID       string `json:"run_id"`
	SegmentID   string `json:"segment_id"`
}

type interactionResumeInputWire struct {
	ItemID  string                        `json:"item_id"`
	Content []interactionContentBlockWire `json:"content"`
}

type interactionContentBlockWire struct {
	Kind      transcript.ContentKind `json:"kind"`
	Text      string                 `json:"text,omitempty"`
	MediaType string                 `json:"media_type,omitempty"`
	Data      string                 `json:"data,omitempty"`
}

type interactionCheckpointState struct {
	modelProvider       *modeladapter.ProviderCapture
	bindings            []plugin.Activation
	resources           []plugin.Dependency
	toolProposals       []plugins.ContributionBinding
	modelInputs         []plugins.ContributionBinding
	modelContexts       []plugins.ContributionBinding
	modelObservers      []plugins.ContributionBinding
	skillDiscoveries    map[domaintool.Group]string
	mcpManifests        map[domaintool.Group][]domaintool.Ref
	options             corechat.Options
	toolMetadata        map[string]toolCallMetadata
	tree                scoperun.Log
	handoff             interactionHandoff
	contextByProcess    map[scoperun.ProcessID]ModelContextTokenCalibration
	instructions        []corechat.Message
	pendingSteers       map[scoperun.SignalID]runs.PreparedSteer
	pendingResumeInputs []runs.CommittedUserInput
}

func (i interactionCheckpointState) restoredStart(continuation runs.ExecutionContinuation) runs.RootExecutionStart {
	return runs.RootExecutionStart{
		SessionID:                continuation.SessionID,
		RunID:                    continuation.RootRunID,
		CWD:                      continuation.Workspace,
		WorkspaceCWD:             continuation.Workspace,
		Isolated:                 continuation.Isolated,
		GoalIncarnationID:        continuation.GoalIncarnationID,
		ModelSelection:           continuation.RootModelSelection(),
		InterruptTypes:           slices.Clone(continuation.Capabilities.InterruptTypes),
		ChildRunAdmissionEnabled: continuation.Capabilities.ChildRuns,
		WorkingContext:           cloneChatMessages(i.instructions),
		Options:                  new(i.options.Clone()),
	}
}

func (i *interactionSession) executorCheckpoint(
	tree scoperun.Log,
) (runs.ExecutorCheckpoint, error) {
	payload, err := i.interactionCheckpointPayload(tree)
	if err != nil {
		return runs.ExecutorCheckpoint{}, err
	}
	if _, err := decodeInteractionCheckpointPayload(payload); err != nil {
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

func encodeInteractionCheckpointPayload(
	tree scoperun.Log,
	contextByProcess map[scoperun.ProcessID]ModelContextTokenCalibration,
	instructions []corechat.Message,
	pendingSteers map[scoperun.SignalID]runs.PreparedSteer,
	pendingResumeInputs []runs.CommittedUserInput,
	toolCalls []toolCallCapture,
	options corechat.Options,
	mcpManifests map[domaintool.Group][]string,
	skillDiscoveries map[domaintool.Group]string,
	handoff *interactionHandoffWire,
	observers []plugins.ContributionBinding,
	contexts []plugins.ContributionBinding,
	inputs []plugins.ContributionBinding,
	toolProposals []plugins.ContributionBinding,
	provider *modeladapter.ProviderCapture,
) ([]byte, error) {
	if err := tree.Validate(); err != nil {
		return nil, errors.New("execution: encode invalid Interaction tree checkpoint")
	}
	frozenInstructions := cloneChatMessages(instructions)
	if frozenInstructions == nil {
		frozenInstructions = []corechat.Message{}
	}
	wire := interactionCheckpointPayloadWire{
		ToolProposals:    new(contributionBindingsWire(toolProposals)),
		ModelObservers:   new(contributionBindingsWire(observers)),
		ModelContexts:    new(contributionBindingsWire(contexts)),
		ModelInputs:      new(contributionBindingsWire(inputs)),
		MCPManifests:     new(mcpManifests),
		SkillDiscoveries: skillDiscoveries,
		ToolCalls:        toolCalls, Options: new(options.Clone()),
		Instructions: new(frozenInstructions),
	}
	providerJSON, err := encodeModelProvider(provider)
	if err != nil {
		return nil, err
	}
	wire.ModelProvider = providerJSON
	wire.Handoff = handoff
	if _, err := interactionInstructionContext(*wire.Instructions); err != nil {
		return nil, err
	}
	wire.Contexts, err = encodeInteractionModelContexts(contextByProcess, handoff.Processes)
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
	// Map iteration must not change the identity of the same captured checkpoint.
	payload, err := json.Marshal(wire, json.Deterministic(true))
	if err != nil {
		return nil, fmt.Errorf("execution: encode Interaction checkpoint: %w", err)
	}
	return payload, nil
}

func encodeInteractionModelContexts(
	contexts map[scoperun.ProcessID]ModelContextTokenCalibration,
	processes []interactionProgressWire,
) ([]interactionModelContextWire, error) {
	retained := make(map[string]struct{}, len(processes))
	for _, process := range processes {
		retained[process.ProcessID] = struct{}{}
	}
	values := make([]interactionModelContextWire, 0, len(contexts))
	for processID, calibration := range contexts {
		if _, found := retained[processID.String()]; !found {
			continue
		}
		if !processID.Valid() {
			return nil, errors.New("model context calibration has an invalid Process identity")
		}
		if err := calibration.Validate(); err != nil {
			return nil, err
		}
		if calibration == (ModelContextTokenCalibration{}) {
			continue
		}
		values = append(values, interactionModelContextWire{
			ProcessID:       processID.String(),
			ReportedTokens:  calibration.ReportedTokens(),
			EstimatedTokens: calibration.EstimatedTokens(),
		})
	}
	slices.SortFunc(values, func(left, right interactionModelContextWire) int {
		return strings.Compare(left.ProcessID, right.ProcessID)
	})
	return values, nil
}

func encodeInteractionPendingSteers(
	pending map[scoperun.SignalID]runs.PreparedSteer,
) ([]interactionPendingSteerWire, error) {
	values := make([]interactionPendingSteerWire, 0, len(pending))
	for _, steer := range pending {
		content, err := encodeInteractionInputContent(steer.Input())
		if err != nil {
			return nil, fmt.Errorf("pending steer %q: %w", steer.ItemID(), err)
		}
		value := interactionPendingSteerWire{ItemID: steer.ItemID(), Content: content}
		if command, bound := steer.Command(); bound {
			value.Command = &interactionSteerCommandWire{
				Key: command.Key, Fingerprint: command.Fingerprint,
				SessionID: command.SessionID, RunID: command.RunID, SegmentID: command.SegmentID,
			}
		}
		values = append(values, value)
	}
	slices.SortFunc(values, func(left, right interactionPendingSteerWire) int {
		return strings.Compare(left.ItemID, right.ItemID)
	})
	return values, nil
}

func encodeInteractionResumeInputs(inputs []runs.CommittedUserInput) ([]interactionResumeInputWire, error) {
	values := make([]interactionResumeInputWire, len(inputs))
	for index, input := range inputs {
		content, err := encodeInteractionInputContent(input.Content)
		if err != nil {
			return nil, fmt.Errorf("Resume input %q: %w", input.ItemID, err)
		}
		values[index] = interactionResumeInputWire{ItemID: input.ItemID, Content: content}
	}
	return values, nil
}

func encodeInteractionInputContent(content []transcript.ContentBlock) ([]interactionContentBlockWire, error) {
	values := make([]interactionContentBlockWire, len(content))
	for index, block := range content {
		value, err := encodeInteractionContentBlock(block)
		if err != nil {
			return nil, fmt.Errorf("content %d: %w", index, err)
		}
		values[index] = value
	}
	return values, nil
}

func encodeInteractionContentBlock(
	block transcript.ContentBlock,
) (interactionContentBlockWire, error) {
	if err := block.Validate(); err != nil {
		return interactionContentBlockWire{}, err
	}
	switch block.Kind {
	case transcript.TextContent:
		return interactionContentBlockWire{Kind: block.Kind, Text: block.Text}, nil
	case transcript.ImageContent:
		return interactionContentBlockWire{
			Kind: block.Kind, MediaType: block.MediaType,
			Data: base64.StdEncoding.EncodeToString(block.Bytes),
		}, nil
	default:
		return interactionContentBlockWire{}, fmt.Errorf("has unknown kind %q", block.Kind)
	}
}

func decodeInteractionCheckpointPayload(payload []byte) (interactionCheckpointState, error) {
	wire, err := decodeInteractionCheckpointWire(payload)
	if err != nil {
		return interactionCheckpointState{}, err
	}
	provider, err := decodeModelProvider(wire.ModelProvider)
	if err != nil {
		return interactionCheckpointState{}, err
	}
	if wire.ToolProposals == nil {
		return interactionCheckpointState{}, errors.New("execution: checkpoint Tool proposal bindings are required")
	}
	proposalContract, err := pluginsdk.ToolProposalContract()
	if err != nil {
		return interactionCheckpointState{}, err
	}
	proposals, err := decodeContributionBindings(*wire.ToolProposals, proposalContract.Identity())
	if err != nil {
		return interactionCheckpointState{}, err
	}
	if wire.MCPManifests == nil {
		return interactionCheckpointState{}, errors.New("execution: checkpoint MCP manifests are required")
	}
	if wire.ModelObservers == nil {
		return interactionCheckpointState{}, errors.New("execution: frozen model observers are required")
	}
	observerContract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		return interactionCheckpointState{}, err
	}
	observers, err := decodeContributionBindings(*wire.ModelObservers, observerContract.Identity())
	if err != nil {
		return interactionCheckpointState{}, err
	}
	if wire.Options == nil || wire.Instructions == nil {
		return interactionCheckpointState{}, errors.New("execution: frozen options and instructions are required")
	}
	manifests, err := decodeCapturedToolGroups(*wire.MCPManifests, wire.SkillDiscoveries)
	if err != nil {
		return interactionCheckpointState{}, err
	}
	if err := wire.Options.Validate(); err != nil {
		return interactionCheckpointState{}, err
	}
	if wire.ModelInputs == nil {
		return interactionCheckpointState{}, errors.New("execution: checkpoint model input bindings are required")
	}
	inputContract, err := pluginsdk.ModelInputContract()
	if err != nil {
		return interactionCheckpointState{}, err
	}
	inputs, err := decodeContributionBindings(*wire.ModelInputs, inputContract.Identity())
	if err != nil {
		return interactionCheckpointState{}, err
	}
	if wire.ModelContexts == nil {
		return interactionCheckpointState{}, errors.New("execution: checkpoint model context bindings are required")
	}
	contextContract, err := pluginsdk.ModelContextContract()
	if err != nil {
		return interactionCheckpointState{}, err
	}
	contexts, err := decodeContributionBindings(*wire.ModelContexts, contextContract.Identity())
	if err != nil {
		return interactionCheckpointState{}, err
	}
	handoff, processes, err := decodeInteractionHandoff(wire.Handoff)
	if err != nil {
		return interactionCheckpointState{}, fmt.Errorf("execution: Interaction checkpoint handoff: %w", err)
	}
	instructions, err := decodeInteractionCheckpointInstructions(*wire.Instructions)
	if err != nil {
		return interactionCheckpointState{}, fmt.Errorf("execution: Interaction checkpoint instructions: %w", err)
	}
	contextByProcess, err := decodeInteractionModelContexts(wire.Contexts, processes)
	if err != nil {
		return interactionCheckpointState{}, fmt.Errorf("execution: Interaction checkpoint model contexts: %w", err)
	}
	pendingSteers, err := decodeInteractionPendingSteers(wire.PendingSteers)
	if err != nil {
		return interactionCheckpointState{}, fmt.Errorf("execution: Interaction checkpoint pending steers: %w", err)
	}
	pendingResumeInputs, err := decodeInteractionResumeInputs(wire.PendingResumeInputs)
	if err != nil {
		return interactionCheckpointState{}, fmt.Errorf("execution: Interaction checkpoint pending Resume inputs: %w", err)
	}
	metadata, err := decodeToolCalls(wire.ToolCalls, processes)
	if err != nil {
		return interactionCheckpointState{}, err
	}
	return interactionCheckpointState{
		modelProvider:    provider,
		toolProposals:    proposals,
		modelObservers:   observers,
		modelContexts:    contexts,
		modelInputs:      inputs,
		mcpManifests:     manifests,
		skillDiscoveries: wire.SkillDiscoveries,
		handoff:          handoff,
		contextByProcess: contextByProcess, instructions: instructions, options: wire.Options.Clone(), pendingSteers: pendingSteers,
		pendingResumeInputs: pendingResumeInputs, toolMetadata: metadata,
	}, nil
}

func decodeInteractionCheckpointWire(payload []byte) (interactionCheckpointPayloadWire, error) {
	var wire interactionCheckpointPayloadWire
	if err := json.Unmarshal(payload, &wire, json.RejectUnknownMembers(true)); err != nil {
		return interactionCheckpointPayloadWire{}, fmt.Errorf("execution: decode Interaction checkpoint: %w", err)
	}
	return wire, nil
}

func decodeInteractionCheckpointInstructions(messages []corechat.Message) ([]corechat.Message, error) {
	instructions := cloneChatMessages(messages)
	canonical, err := interactionInstructionContext(instructions)
	if err != nil || len(canonical) != len(instructions) {
		if err == nil {
			err = errors.New("instruction context contains a non-system message")
		}
		return nil, err
	}
	return instructions, nil
}

func decodeInteractionModelContexts(
	values []interactionModelContextWire,
	processes map[scoperun.ProcessID]struct{},
) (map[scoperun.ProcessID]ModelContextTokenCalibration, error) {
	contexts := make(map[scoperun.ProcessID]ModelContextTokenCalibration, len(values))
	previous := ""
	for index, value := range values {
		if index > 0 && value.ProcessID <= previous {
			return nil, errors.New("model context calibrations are not in canonical order")
		}
		processID, err := scoperun.ParseProcessID(value.ProcessID)
		if err != nil {
			return nil, fmt.Errorf("model context Process identity: %w", err)
		}
		if _, found := processes[processID]; !found {
			return nil, errors.New("model context names a foreign Process")
		}
		calibration, err := NewModelContextTokenCalibration(
			value.ReportedTokens,
			value.EstimatedTokens,
		)
		if err != nil {
			return nil, err
		}
		contexts[processID] = calibration
		previous = value.ProcessID
	}
	return contexts, nil
}

func decodeInteractionPendingSteers(
	values []interactionPendingSteerWire,
) (map[scoperun.SignalID]runs.PreparedSteer, error) {
	pending := make(map[scoperun.SignalID]runs.PreparedSteer, len(values))
	previous := ""
	for _, value := range values {
		if value.ItemID <= previous {
			return nil, errors.New("pending steer Items are not in canonical order")
		}
		steer, err := value.decode()
		if err != nil {
			return nil, fmt.Errorf("pending steer %q: %w", value.ItemID, err)
		}
		signalID, err := steerSignalID(steer.ItemID())
		if err != nil {
			return nil, err
		}
		pending[signalID] = steer
		previous = value.ItemID
	}
	return pending, nil
}

func (w interactionPendingSteerWire) decode() (runs.PreparedSteer, error) {
	content, err := decodeInteractionInputContent(w.Content)
	if err != nil {
		return runs.PreparedSteer{}, err
	}
	if w.Command == nil {
		return runs.PrepareLocalSteer(w.ItemID, content)
	}
	binding := runs.SteerCommandBinding{
		CommandIdentity: runs.CommandIdentity{Key: w.Command.Key, Fingerprint: w.Command.Fingerprint},
		SessionID:       w.Command.SessionID, RunID: w.Command.RunID, SegmentID: w.Command.SegmentID,
	}
	steer, err := binding.Prepare(content)
	if err != nil {
		return runs.PreparedSteer{}, err
	}
	if steer.ItemID() != w.ItemID {
		return runs.PreparedSteer{}, errors.New("pending steer command nominates another Item")
	}
	return steer, nil
}

func decodeInteractionResumeInputs(values []interactionResumeInputWire) ([]runs.CommittedUserInput, error) {
	inputs := make([]runs.CommittedUserInput, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if err := identity.ValidateItem(value.ItemID); err != nil {
			return nil, fmt.Errorf("Resume input %d: %w", index, err)
		}
		if _, duplicate := seen[value.ItemID]; duplicate {
			return nil, fmt.Errorf("repeated Resume input %q", value.ItemID)
		}
		seen[value.ItemID] = struct{}{}
		content, err := decodeInteractionInputContent(value.Content)
		if err != nil {
			return nil, fmt.Errorf("Resume input %q: %w", value.ItemID, err)
		}
		if _, err := runs.MaterializeUserMessage(content); err != nil {
			return nil, fmt.Errorf("Resume input %q: %w", value.ItemID, err)
		}
		inputs[index] = runs.CommittedUserInput{ItemID: value.ItemID, Content: content}
	}
	return inputs, nil
}

func decodeInteractionInputContent(values []interactionContentBlockWire) ([]transcript.ContentBlock, error) {
	content := make([]transcript.ContentBlock, len(values))
	for index, block := range values {
		value, err := block.decode()
		if err != nil {
			return nil, fmt.Errorf("content %d: %w", index, err)
		}
		content[index] = value
	}
	return content, nil
}

func (w interactionContentBlockWire) decode() (transcript.ContentBlock, error) {
	switch w.Kind {
	case transcript.TextContent:
		return w.decodeText()
	case transcript.ImageContent:
		return w.decodeImage()
	default:
		return transcript.ContentBlock{}, fmt.Errorf("has unknown kind %q", w.Kind)
	}
}

func (w interactionContentBlockWire) decodeText() (transcript.ContentBlock, error) {
	if w.Text == "" || w.MediaType != "" || w.Data != "" {
		return transcript.ContentBlock{}, errors.New("is not canonical text")
	}
	content := transcript.ContentBlock{Kind: transcript.TextContent, Text: w.Text}
	if err := content.Validate(); err != nil {
		return transcript.ContentBlock{}, err
	}
	return content, nil
}

func (w interactionContentBlockWire) decodeImage() (transcript.ContentBlock, error) {
	if w.Text != "" || w.MediaType == "" || w.Data == "" {
		return transcript.ContentBlock{}, errors.New("is not canonical image")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(w.Data)
	if err != nil {
		return transcript.ContentBlock{}, fmt.Errorf("image data: %w", err)
	}
	if base64.StdEncoding.EncodeToString(data) != w.Data {
		return transcript.ContentBlock{}, errors.New("image data is not canonical base64")
	}
	content := transcript.ContentBlock{
		Kind: transcript.ImageContent, MediaType: w.MediaType, Bytes: data,
	}
	if err := content.Validate(); err != nil {
		return transcript.ContentBlock{}, err
	}
	return content, nil
}
