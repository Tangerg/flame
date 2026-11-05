package execution

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

const (
	preparationMetadataKey = "flame.execution.preparation"
	steerMetadataKey       = "flame.execution.steer"
)

// These coordinates project committed Scope operations into Flame's call
// accounting. Preparation and model dispatch have distinct effect identities.
type modelCallSource struct {
	process  scoperun.ProcessID
	effect   scoperun.EffectID
	program  scoperun.ProgramRef
	sequence uint64
	steers   []scoperun.SignalID
}

func (s modelCallSource) valid() bool {
	return s.process.Valid() && s.effect.Valid() && s.effect.Process == s.process && s.program.Valid() && s.sequence > 0
}

type preparedModelReference struct {
	Process  scoperun.ProcessID `json:"process"`
	Effect   scoperun.EffectID  `json:"effect"`
	Sequence uint64             `json:"sequence"`
}

// Frame's caller identity is flat and bounded, while a Scope Effect contains an
// arbitrarily nested process path. The exact original remains in Scope's log.
func effectInvocationParent(effect scoperun.EffectID) string {
	return "effect:" + scoperun.ComputeDigest([]byte(effect.String())).String()
}

func (r preparedModelReference) valid() bool {
	return r.Process.Valid() && r.Effect.Valid() && r.Effect.Process == r.Process && r.Sequence > 0
}

func modelPreparationReference(messages []corechat.Message) (preparedModelReference, int, error) {
	var reference preparedModelReference
	index := -1
	for position := range messages {
		candidate, found, err := messages[position].Metadata.Decode[preparedModelReference](preparationMetadataKey)
		if err != nil {
			return reference, -1, err
		}
		if !found {
			continue
		}
		if index >= 0 || !candidate.valid() {
			return reference, -1, errors.New("execution: model request has ambiguous preparation ownership")
		}
		reference, index = candidate, position
	}
	if index < 0 {
		return reference, -1, errors.New("execution: model request has no committed preparation reference")
	}
	return reference, index, nil
}

func withoutPreparationReference(messages []corechat.Message) []corechat.Message {
	messages = cloneChatMessages(messages)
	for index := range messages {
		delete(messages[index].Metadata, preparationMetadataKey)
		if len(messages[index].Metadata) == 0 {
			messages[index].Metadata = nil
		}
	}
	return messages
}

func providerMessages(messages []corechat.Message) []corechat.Message {
	messages = withoutPreparationReference(messages)
	for index := range messages {
		delete(messages[index].Metadata, steerMetadataKey)
		if len(messages[index].Metadata) == 0 {
			messages[index].Metadata = nil
		}
	}
	return messages
}

func (i *interactionSession) appliedSteers(messages []corechat.Message) ([]scoperun.SignalID, error) {
	var result []scoperun.SignalID
	seen := make(map[scoperun.SignalID]struct{})
	for _, message := range messages {
		id, found, err := message.Metadata.Decode[scoperun.SignalID](steerMetadataKey)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		if !id.Valid() {
			return nil, errors.New("execution: context contains an invalid steer identity")
		}
		i.state.mu.Lock()
		_, pending := i.state.pendingSteers[id]
		i.state.mu.Unlock()
		if !pending {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, errors.New("execution: context repeats one pending steer")
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

func (i *interactionSession) modelSource(ctx context.Context, request *corechat.Request) (modelCallSource, error) {
	if request == nil {
		return modelCallSource{}, errors.New("execution: model request is nil")
	}
	reference, _, err := modelPreparationReference(request.Messages)
	if err != nil {
		return modelCallSource{}, err
	}
	history, err := i.committedTree(ctx, reference.Process.Root())
	if err != nil {
		return modelCallSource{}, err
	}
	inspection, err := i.logCache.inspect(history)
	if err != nil {
		return modelCallSource{}, err
	}
	owner, found := inspection.Process(reference.Process)
	if !found {
		return modelCallSource{}, errors.New("execution: model preparation has no committed process")
	}
	for _, pending := range owner.PendingEffects {
		candidate, model, err := interaction.ModelRequestFromEffect(pending.Request.Effect())
		if err != nil {
			return modelCallSource{}, err
		}
		if !model {
			continue
		}
		left, err := json.Marshal(candidate, json.Deterministic(true))
		if err != nil {
			return modelCallSource{}, err
		}
		right, err := json.Marshal(request, json.Deterministic(true))
		if err != nil || !bytes.Equal(left, right) {
			return modelCallSource{}, errors.Join(errors.New("execution: physical request differs from committed model intent"), err)
		}
		steers, err := i.appliedSteers(request.Messages)
		if err != nil {
			return modelCallSource{}, err
		}
		return modelCallSource{process: reference.Process, effect: pending.Request.ID(), program: owner.Program,
			sequence: reference.Sequence, steers: steers}, nil
	}
	return modelCallSource{}, errors.New("execution: model call has no committed effect")
}

type toolCallSource struct {
	process  scoperun.ProcessID
	sequence uint64
	index    uint32
}

// Scope admits an explicit idempotency key to each Tool. The canonical pending
// request owns its coordinates; the committed model response owns its ordinal.
func (i *interactionSession) toolSource(ctx context.Context, bound toolcontract.Invocation) (interaction.ToolInvocation, toolCallSource, error) {
	history, err := i.committedTree(ctx, i.processRootID())
	if err != nil {
		return interaction.ToolInvocation{}, toolCallSource{}, err
	}
	inspection, err := i.logCache.inspect(history)
	if err != nil {
		return interaction.ToolInvocation{}, toolCallSource{}, err
	}
	var invocation interaction.ToolInvocation
	for _, process := range inspection.Processes {
		if _, interactionProcess := i.state.deployments.interactions[process.Program]; !interactionProcess {
			continue
		}
		for _, pending := range process.PendingEffects {
			_, tool, err := interaction.ToolCallFromEffect(pending.Request.Effect())
			if err != nil {
				return invocation, toolCallSource{}, err
			}
			if !tool {
				continue
			}
			candidate, err := interaction.ToolInvocationFromRequest(pending.Request)
			if err != nil {
				return invocation, toolCallSource{}, err
			}
			if candidate.IdempotencyKey() == bound.IdempotencyKey() {
				if invocation.Valid() {
					return invocation, toolCallSource{}, errors.New("execution: Tool idempotency key has multiple pending owners")
				}
				invocation = candidate
			}
		}
	}
	if !invocation.Valid() {
		return invocation, toolCallSource{}, errors.New("execution: Tool call has no committed invocation")
	}
	source, err := toolProposalSource(history, invocation.Reference(), invocation.ToolCall())
	return invocation, source, err
}

func toolProposalSource(history scoperun.Log, reference scoperun.EffectID, call corechat.ToolCall) (toolCallSource, error) {
	var foundReference bool
	settlements := make(map[scoperun.EffectID]scoperun.Settlement)
	for _, entry := range slices.Backward(history.Entries) {
		switch record := entry.Record.(type) {
		case scoperun.EffectSettled:
			if foundReference && record.ID.Process == reference.Process {
				settlements[record.ID] = record.Outcome
			}
		case scoperun.EffectIntent:
			if record.ID == reference {
				foundReference = true
				continue
			}
			if !foundReference || record.ID.Process != reference.Process {
				continue
			}
			request, model, err := interaction.ModelRequestFromEffect(record.Effect)
			if err != nil {
				return toolCallSource{}, err
			}
			if !model {
				continue
			}
			preparation, _, err := modelPreparationReference(request.Messages)
			if err != nil {
				return toolCallSource{}, err
			}
			settled, err := interaction.ModelSettlementFromOutcome(settlements[record.ID])
			if err != nil {
				return toolCallSource{}, err
			}
			return proposalCallSource(reference.Process, preparation.Sequence, settled.Response, call)
		}
	}
	return toolCallSource{}, fmt.Errorf("execution: missing original Tool effect %s", reference)
}

func proposalCallSource(process scoperun.ProcessID, sequence uint64, response *corechat.Response, call corechat.ToolCall) (toolCallSource, error) {
	if response == nil || response.Output == nil || response.Output.Message == nil {
		return toolCallSource{}, errors.New("execution: Tool intent has no committed model proposal")
	}
	var index uint32
	for _, part := range response.Output.Message.Parts {
		if part.ToolCall == nil {
			continue
		}
		if *part.ToolCall == call {
			return toolCallSource{process: process, sequence: sequence, index: index}, nil
		}
		index++
	}
	return toolCallSource{}, errors.New("execution: Tool intent differs from its model proposal")
}
