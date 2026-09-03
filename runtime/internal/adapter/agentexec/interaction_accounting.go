package agentexec

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

// interactionAccounting owns per-Process model usage, usage restored from
// retired Processes, and the root's Tool-call count. These facts share the
// accounting snapshot/checkpoint invariant but not the Process-tree lock.
type interactionAccounting struct {
	mu                       sync.Mutex
	usageByProcess           map[agent.ProcessID]map[string]accounting.ModelUsage
	carriedUsage             map[string]accounting.ModelUsage
	contextByProcess         map[agent.ProcessID]ModelContextTokenCalibration
	preparedContextByProcess map[agent.ProcessID]preparedModelContext
	selection                modelref.Selection
	pricing                  accounting.Pricing
	toolCalls                int
}

type preparedModelContext struct {
	effectID  agent.EffectID
	sequence  uint32
	estimated int
}

type modelCallAccountingInput struct {
	message       corechat.Message
	delta         accounting.ModelUsage
	contextTokens int64
}

func newInteractionAccounting(
	selection modelref.Selection,
	pricing accounting.Pricing,
) interactionAccounting {
	return interactionAccounting{
		usageByProcess:           make(map[agent.ProcessID]map[string]accounting.ModelUsage),
		carriedUsage:             make(map[string]accounting.ModelUsage),
		contextByProcess:         make(map[agent.ProcessID]ModelContextTokenCalibration),
		preparedContextByProcess: make(map[agent.ProcessID]preparedModelContext),
		selection:                selection,
		pricing:                  pricing,
	}
}

func (i *interactionAccounting) modelContextCalibration(
	invocation interaction.ModelInvocation,
) ModelContextTokenCalibration {
	if i == nil || !invocation.Valid() {
		return ModelContextTokenCalibration{}
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.contextByProcess[invocation.Relation().ProcessID()]
}

func (i *interactionAccounting) prepareModelContext(
	invocation interaction.ModelInvocation,
	estimated int,
) error {
	if i == nil || !invocation.Valid() || estimated <= 0 {
		return errors.New("agentexec: prepare model context requires valid attribution and estimate")
	}
	processID := invocation.Relation().ProcessID()
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, exists := i.preparedContextByProcess[processID]; exists {
		return fmt.Errorf("agentexec: Process %s already has a prepared model context", processID)
	}
	i.preparedContextByProcess[processID] = preparedModelContext{
		effectID: invocation.EffectID(), sequence: invocation.ModelCallSequence(), estimated: estimated,
	}
	return nil
}

// discardPreparedModelContext releases the one-call calibration reservation
// when that exact invocation exits without successful accounting. Identity is
// matched under the lock so a stale exit cannot discard a later call's slot.
func (i *interactionAccounting) discardPreparedModelContext(invocation interaction.ModelInvocation) {
	if i == nil || !invocation.Valid() {
		return
	}
	processID := invocation.Relation().ProcessID()
	i.mu.Lock()
	prepared, found := i.preparedContextByProcess[processID]
	if found && prepared.effectID == invocation.EffectID() &&
		prepared.sequence == invocation.ModelCallSequence() {
		delete(i.preparedContextByProcess, processID)
	}
	i.mu.Unlock()
}

func (i *interactionAccounting) providerName() string { return i.selection.Provider() }

func (i *interactionAccounting) modelName() string { return i.selection.Model() }

func (i *interactionAccounting) recordToolCall() {
	i.mu.Lock()
	i.toolCalls++
	i.mu.Unlock()
}

func (i *interactionAccounting) toolCallCount() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.toolCalls
}

func (i *interactionAccounting) snapshot() (accounting.Snapshot, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	byModel := make(map[string]accounting.ModelUsage)
	if err := mergeInteractionUsage(byModel, i.carriedUsage); err != nil {
		return accounting.Snapshot{}, err
	}
	for _, processUsage := range i.usageByProcess {
		if err := mergeInteractionUsage(byModel, processUsage); err != nil {
			return accounting.Snapshot{}, err
		}
	}
	return interactionUsageSnapshot(byModel), nil
}

func interactionUsageSnapshot(byModel map[string]accounting.ModelUsage) accounting.Snapshot {
	models := make([]accounting.ModelUsage, 0, len(byModel))
	for _, usage := range byModel {
		models = append(models, usage)
	}
	slices.SortFunc(models, func(left, right accounting.ModelUsage) int {
		return strings.Compare(left.Model, right.Model)
	})
	return accounting.Snapshot{Models: models}
}

func mergeInteractionUsage(
	target map[string]accounting.ModelUsage,
	source map[string]accounting.ModelUsage,
) error {
	for model, usage := range source {
		current, found := target[model]
		if !found {
			target[model] = usage
			continue
		}
		combined, err := current.Add(usage)
		if err != nil {
			return fmt.Errorf("agentexec: aggregate model %q usage: %w", model, err)
		}
		target[model] = combined
	}
	return nil
}

func advanceProcessUsage(
	current map[string]accounting.ModelUsage,
	delta accounting.ModelUsage,
	expectedCalls uint32,
) (map[string]accounting.ModelUsage, []accounting.ModelUsage, accounting.ModelUsage, error) {
	next := maps.Clone(current)
	if next == nil {
		next = make(map[string]accounting.ModelUsage)
	}
	model, found := next[delta.Model]
	if !found {
		model = delta
	} else {
		var err error
		model, err = model.Add(delta)
		if err != nil {
			return nil, nil, accounting.ModelUsage{}, fmt.Errorf("agentexec: aggregate model call usage: %w", err)
		}
	}
	if err := model.Validate(); err != nil {
		return nil, nil, accounting.ModelUsage{}, fmt.Errorf("agentexec: aggregate model call: %w", err)
	}
	next[delta.Model] = model
	snapshot := interactionUsageSnapshot(next)
	total, err := snapshot.Total()
	if err != nil {
		return nil, nil, accounting.ModelUsage{}, fmt.Errorf("agentexec: total model usage: %w", err)
	}
	if total.Calls != int(expectedCalls) {
		return nil, nil, accounting.ModelUsage{}, fmt.Errorf(
			"agentexec: model call sequence %d differs from accounted calls %d",
			expectedCalls, total.Calls,
		)
	}
	return next, snapshot.Models, total, nil
}

func (i *interactionAccounting) restore(
	usageByProcess map[agent.ProcessID]map[string]accounting.ModelUsage,
	carriedUsage map[string]accounting.ModelUsage,
	contextByProcess map[agent.ProcessID]ModelContextTokenCalibration,
) {
	i.mu.Lock()
	i.usageByProcess = usageByProcess
	i.carriedUsage = carriedUsage
	i.contextByProcess = contextByProcess
	i.preparedContextByProcess = make(map[agent.ProcessID]preparedModelContext)
	i.mu.Unlock()
}

func (i *interactionAccounting) checkpointLocked() (
	map[agent.ProcessID]map[string]accounting.ModelUsage,
	map[string]accounting.ModelUsage,
	map[agent.ProcessID]ModelContextTokenCalibration,
) {
	usageByProcess := make(map[agent.ProcessID]map[string]accounting.ModelUsage, len(i.usageByProcess))
	for processID, byModel := range i.usageByProcess {
		usageByProcess[processID] = maps.Clone(byModel)
	}
	return usageByProcess, maps.Clone(i.carriedUsage), maps.Clone(i.contextByProcess)
}

func (i *interactionSession) interactionCheckpointPayload(
	tree agent.TreeSnapshot,
) ([]byte, error) {
	// Accounting and pending inputs were one lock domain before P113. Hold both
	// owners while copying so the checkpoint retains the same atomic snapshot,
	// without making every model call contend with Process lifecycle transitions.
	i.accounting.mu.Lock()
	i.state.mu.Lock()
	usageByProcess, carried, contexts := i.accounting.checkpointLocked()
	pendingSteers := make(map[agent.SignalID]pendingInteractionSteer, len(i.state.pendingSteers))
	for signalID, pending := range i.state.pendingSteers {
		pendingSteers[signalID] = pendingInteractionSteer{
			content: transcript.CloneContent(pending.content),
		}
	}
	var pendingContinuation *pendingInteractionContinuation
	if pending := i.state.pendingContinuation; pending != nil {
		pendingContinuation = &pendingInteractionContinuation{
			processID: pending.processID,
			itemID:    pending.itemID,
			content:   transcript.CloneContent(pending.content),
		}
	}
	i.state.mu.Unlock()
	i.accounting.mu.Unlock()

	instructions, err := interactionInstructionContext(i.start.WorkingContext)
	if err != nil {
		return nil, err
	}
	return encodeInteractionCheckpointPayload(
		tree,
		usageByProcess,
		carried,
		contexts,
		instructions,
		pendingSteers,
		pendingContinuation,
	)
}

func (i *interactionAccounting) accountModelCall(
	invocation interaction.ModelInvocation,
	callID string,
	response *corechat.Response,
) (runs.ModelCallCompleted, error) {
	input, err := newModelCallAccountingInput(response, i.selection, i.pricing)
	if err != nil {
		return runs.ModelCallCompleted{}, err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.accountModelCallLocked(invocation, callID, input)
}

func newModelCallAccountingInput(
	response *corechat.Response,
	selection modelref.Selection,
	pricing accounting.Pricing,
) (modelCallAccountingInput, error) {
	if response == nil || response.Output == nil || response.Output.Message == nil {
		return modelCallAccountingInput{}, errors.New("agentexec: account model call without an assistant message")
	}
	if err := conversation.ValidateMessageIdentities(*response.Output.Message); err != nil {
		return modelCallAccountingInput{}, fmt.Errorf("agentexec: account model call: %w", err)
	}
	delta := modelUsage(response, selection, pricing)
	if err := delta.Validate(); err != nil {
		return modelCallAccountingInput{}, fmt.Errorf("agentexec: account model call: %w", err)
	}
	var contextTokens int64
	if response.Metadata != nil {
		contextTokens = response.Metadata.Usage.InputTokens
	}
	return modelCallAccountingInput{
		message: response.Output.Message.Clone(), delta: delta, contextTokens: contextTokens,
	}, nil
}

func (i *interactionAccounting) accountModelCallLocked(
	invocation interaction.ModelInvocation,
	callID string,
	input modelCallAccountingInput,
) (runs.ModelCallCompleted, error) {
	processID := invocation.Relation().ProcessID()
	prepared, preparedFound := i.preparedContextByProcess[processID]
	if preparedFound && (prepared.effectID != invocation.EffectID() ||
		prepared.sequence != invocation.ModelCallSequence()) {
		return runs.ModelCallCompleted{}, errors.New("agentexec: model response has no matching prepared context")
	}
	nextUsage, models, total, err := advanceProcessUsage(
		i.usageByProcess[processID], input.delta, invocation.ModelCallSequence(),
	)
	if err != nil {
		return runs.ModelCallCompleted{}, err
	}
	calibration, calibrated, err := calibrateModelContext(prepared, preparedFound, input.contextTokens)
	if err != nil {
		return runs.ModelCallCompleted{}, err
	}
	completed := runs.ModelCallCompleted{
		CallID: callID, Message: input.message, TokenUsage: total.TokenUsage,
		ByModel: slices.Clone(models), Cost: total.Cost, Steps: total.Calls,
		ContextTokens: input.contextTokens,
	}
	i.usageByProcess[processID] = nextUsage
	if preparedFound {
		delete(i.preparedContextByProcess, processID)
	}
	if calibrated {
		i.contextByProcess[processID] = calibration
	}
	return completed, nil
}

func calibrateModelContext(
	prepared preparedModelContext,
	found bool,
	reported int64,
) (ModelContextTokenCalibration, bool, error) {
	if !found || reported <= 0 {
		return ModelContextTokenCalibration{}, false, nil
	}
	calibration, err := NewModelContextTokenCalibration(reported, prepared.estimated)
	if err != nil {
		return ModelContextTokenCalibration{}, false, err
	}
	return calibration, true, nil
}

func (i *interactionAccounting) segmentUsage(processID agent.ProcessID) (*runs.SegmentUsage, error) {
	i.mu.Lock()
	snapshot := interactionUsageSnapshot(i.usageByProcess[processID])
	i.mu.Unlock()
	if len(snapshot.Models) == 0 {
		return nil, nil
	}
	total, err := snapshot.Total()
	if err != nil {
		return nil, fmt.Errorf("agentexec: total segment usage: %w", err)
	}
	return &runs.SegmentUsage{
		Tokens: total.TokenUsage, ByModel: snapshot.Models,
		Cost: total.Cost, Steps: total.Calls,
	}, nil
}
