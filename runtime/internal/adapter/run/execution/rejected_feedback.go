package execution

import (
	"context"
	"errors"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	"github.com/Tangerg/scope/core/chat"
)

// Scope's context request contains its committed local rejection feedback.
// These proposals never emitted external effects, so SettledResults correctly
// omits them. Publish their exact conversation results before another provider
// sees them, without manufacturing an Effect reference or executing a Tool.
func (i *interactionSession) commitRejectedToolFeedback(ctx context.Context, preparation modelCallSource, request *chat.Request) error {
	if preparation.sequence < 2 {
		return nil
	}
	history, err := i.committedTree(ctx, preparation.process.Root())
	if err != nil {
		return err
	}
	calls, settled, err := i.unexecutedProposals(history, preparation.process, preparation.sequence-1)
	if err != nil || len(calls) == 0 {
		return err
	}
	results := latestToolFeedback(request.Messages)
	sequence := preparation.sequence - 1
	var pending []runs.ExecutorEvent
	var modelResults []chat.ToolResult
	for index, call := range calls {
		result, found := results[call.ID]
		if !found || result.Name != call.Name {
			return errors.New("execution: canonical context lost a proposed Tool's feedback")
		}
		modelResults = append(modelResults, result.Clone())
		if settled[uint32(index)] {
			continue
		}
		if !result.IsError {
			return errors.New("execution: unexecuted Tool proposal has successful feedback")
		}
		entry := interaction.ResultEntry{ToolCallIndex: uint32(index), Call: call, Result: result, Disposition: interaction.ResultRejected}
		event, err := i.toolResultProjection(preparation.process, sequence, entry)
		if err != nil {
			return err
		}
		fact := event.Payload.(runs.ToolResultsCommitted)
		committed, err := i.executionTrees.ExecutionResultCommitted(ctx, i.start.SessionID, fact.Publication)
		if err != nil {
			return err
		}
		if !committed {
			pending = append(pending, event)
		}
	}
	for index, event := range pending {
		fact := event.Payload.(runs.ToolResultsCommitted)
		if index == len(pending)-1 {
			fact.ModelResults = modelResults
		}
		if err := i.commitFact(ctx, event.Member, fact); err != nil {
			return err
		}
	}
	return nil
}

func committedModelResponse(history scoperun.Log, process scoperun.ProcessID, sequence uint64) (*chat.Response, error) {
	settlements := make(map[scoperun.EffectID]scoperun.EffectSettled)
	for _, entry := range slices.Backward(history.Entries) {
		switch record := entry.Record.(type) {
		case scoperun.EffectSettled:
			if record.ID.Process == process {
				settlements[record.ID] = record
			}
		case scoperun.EffectIntent:
			if record.ID.Process != process {
				continue
			}
			intent, found, err := interaction.ModelCallFromEffect(record.Effect)
			if err != nil {
				return nil, err
			}
			if !found {
				continue
			}
			if intent.Sequence != sequence {
				continue
			}
			settlement, err := interaction.ModelSettlementFromRecord(settlements[record.ID])
			return settlement.Response, err
		}
	}
	return nil, nil
}

func (i *interactionSession) unexecutedProposals(history scoperun.Log, process scoperun.ProcessID, sequence uint64) ([]chat.ToolCall, map[uint32]bool, error) {
	response, err := committedModelResponse(history, process, sequence)
	if err != nil {
		return nil, nil, err
	}
	if response == nil || response.Output == nil || response.Output.Message == nil {
		return nil, nil, errors.New("execution: context preparation lost its preceding model response")
	}
	var calls []chat.ToolCall
	for _, part := range response.Output.Message.Parts {
		if part.ToolCall != nil {
			calls = append(calls, *part.ToolCall)
		}
	}
	if len(calls) == 0 {
		return nil, nil, nil
	}
	reference, found := capturedProgram(history, process)
	program, owned := i.state.deployments.interactions[reference]
	if !found || !owned {
		return nil, nil, errors.New("execution: proposed Tools have no committed Interaction program")
	}
	rounds, err := program.history.SettledResults(history)
	if err != nil {
		return nil, nil, err
	}
	settled := make(map[uint32]bool)
	for _, round := range rounds {
		if round.ProcessID() == process && round.ModelCallSequence() == sequence {
			for _, entry := range round.Entries() {
				settled[entry.ToolCallIndex] = true
			}
		}
	}
	if len(settled) == len(calls) {
		return nil, nil, nil
	}
	return calls, settled, nil
}

func latestToolFeedback(messages []chat.Message) map[string]chat.ToolResult {
	results := make(map[string]chat.ToolResult)
	for _, message := range messages {
		if message.Role == chat.RoleAssistant {
			clear(results)
		}
		for _, part := range message.Parts {
			if part.ToolResult != nil {
				results[part.ToolResult.ID] = part.ToolResult.Clone()
			}
		}
	}
	return results
}
