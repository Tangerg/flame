package execution

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"

	scoperun "github.com/Tangerg/scope/agent/run"
)

// interactionAccounting retains Scope process checkpoints and context calibration.
// Run consumption advances only through per-call facts, never this snapshot.
type interactionAccounting struct {
	mu                       sync.Mutex
	usageByProcess           map[scoperun.ProcessID]map[string]accounting.ModelUsage
	contextByProcess         map[scoperun.ProcessID]ModelContextTokenCalibration
	preparedContextByProcess map[scoperun.ProcessID]preparedModelContext
	selection                modelchoice.Selection
	toolCalls                int
}

type preparedModelContext struct {
	effectID  scoperun.EffectID
	sequence  uint64
	estimated int
}

func newInteractionAccounting(
	selection modelchoice.Selection,
) interactionAccounting {
	return interactionAccounting{
		usageByProcess:           make(map[scoperun.ProcessID]map[string]accounting.ModelUsage),
		contextByProcess:         make(map[scoperun.ProcessID]ModelContextTokenCalibration),
		preparedContextByProcess: make(map[scoperun.ProcessID]preparedModelContext),
		selection:                selection,
	}
}

func (i *interactionAccounting) modelContextCalibration(
	invocation modelCallSource,
) ModelContextTokenCalibration {
	if !invocation.valid() {
		return ModelContextTokenCalibration{}
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.contextByProcess[invocation.process]
}

func (i *interactionAccounting) prepareModelContext(
	invocation modelCallSource,
	estimated int,
) error {
	if !invocation.valid() || estimated <= 0 {
		return errors.New("execution: prepare model context requires valid attribution and estimate")
	}
	processID := invocation.process
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, exists := i.preparedContextByProcess[processID]; exists {
		return fmt.Errorf("execution: Process %s already has a prepared model context", processID)
	}
	i.preparedContextByProcess[processID] = preparedModelContext{
		effectID: invocation.effect, sequence: invocation.sequence, estimated: estimated,
	}
	return nil
}

// discardPreparedModelContext releases the one-call calibration reservation
// when that exact invocation exits without successful accounting. Identity is
// matched under the lock so a stale exit cannot discard a later call's slot.
func (i *interactionAccounting) discardPreparedModelContext(invocation modelCallSource) {
	if !invocation.valid() {
		return
	}
	processID := invocation.process
	i.mu.Lock()
	prepared, found := i.preparedContextByProcess[processID]
	if found && prepared.sequence == invocation.sequence {
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

func advanceProcessUsage(
	current map[string]accounting.ModelUsage,
	delta accounting.ModelUsage,
	expectedCalls uint64,
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
			return nil, nil, accounting.ModelUsage{}, fmt.Errorf("execution: aggregate model call usage: %w", err)
		}
	}
	if err := model.Validate(); err != nil {
		return nil, nil, accounting.ModelUsage{}, fmt.Errorf("execution: aggregate model call: %w", err)
	}
	next[delta.Model] = model
	snapshot := interactionUsageSnapshot(next)
	total, err := snapshot.Total()
	if err != nil {
		return nil, nil, accounting.ModelUsage{}, fmt.Errorf("execution: total model usage: %w", err)
	}
	if uint64(total.Calls) != expectedCalls {
		return nil, nil, accounting.ModelUsage{}, fmt.Errorf(
			"execution: model call sequence %d differs from accounted calls %d",
			expectedCalls, total.Calls,
		)
	}
	return next, snapshot.Models, total, nil
}

func (i *interactionAccounting) restore(
	usageByProcess map[scoperun.ProcessID]map[string]accounting.ModelUsage,
	contextByProcess map[scoperun.ProcessID]ModelContextTokenCalibration,
) {
	i.mu.Lock()
	i.usageByProcess = usageByProcess
	i.contextByProcess = contextByProcess
	i.mu.Unlock()
}

func (i *interactionAccounting) checkpointLocked() (
	map[scoperun.ProcessID]map[string]accounting.ModelUsage,
	map[scoperun.ProcessID]ModelContextTokenCalibration,
) {
	usageByProcess := make(map[scoperun.ProcessID]map[string]accounting.ModelUsage, len(i.usageByProcess))
	for processID, byModel := range i.usageByProcess {
		usageByProcess[processID] = maps.Clone(byModel)
	}
	return usageByProcess, maps.Clone(i.contextByProcess)
}

func (i *interactionSession) interactionCheckpointPayload(
	tree scoperun.Log,
) ([]byte, error) {
	// Checkpoints need one atomic snapshot across accounting and pending inputs.
	i.accounting.mu.Lock()
	i.state.mu.Lock()
	usageByProcess, contexts := i.accounting.checkpointLocked()
	pendingSteers := make(map[scoperun.SignalID]runs.PreparedSteer, len(i.state.pendingSteers))
	for signalID, pending := range i.state.pendingSteers {
		pendingSteers[signalID] = pending.Clone()
	}
	var pendingContinuation *pendingInteractionContinuation
	if pending := i.state.pendingContinuation; pending != nil {
		pendingContinuation = &pendingInteractionContinuation{
			processID: pending.processID,
			itemID:    pending.itemID,
			content:   transcript.CloneContent(pending.content),
		}
	}
	metadata := make([]toolResultMetadata, 0, len(i.state.toolMetadata))
	for _, value := range i.state.toolMetadata {
		metadata = append(metadata, value.clone())
	}
	slices.SortFunc(metadata, func(left, right toolResultMetadata) int {
		return strings.Compare(left.Start.CallID, right.Start.CallID)
	})
	i.state.mu.Unlock()
	i.accounting.mu.Unlock()

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
		usageByProcess,
		contexts,
		instructions,
		pendingSteers,
		pendingContinuation,
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

func (i *interactionAccounting) accountModelCall(
	invocation modelCallSource,
	consumption runs.ModelCallCompleted,
) (runs.ModelCallCompleted, error) {
	delta := accounting.ModelUsage{Model: consumption.Model, Cost: consumption.Cost, Calls: 1}
	if consumption.ReportedUsage != nil {
		var err error
		delta.Tokens, err = accounting.NewTokens(*consumption.ReportedUsage)
		if err != nil {
			return runs.ModelCallCompleted{}, err
		}
	}
	if err := delta.Validate(); err != nil {
		return runs.ModelCallCompleted{}, fmt.Errorf("execution: account model call: %w", err)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	processID := invocation.process
	prepared, preparedFound := i.preparedContextByProcess[processID]
	if preparedFound && (prepared.sequence != invocation.sequence) {
		return runs.ModelCallCompleted{}, errors.New("execution: model response has no matching prepared context")
	}
	nextUsage, _, _, err := advanceProcessUsage(
		i.usageByProcess[processID], delta, invocation.sequence,
	)
	if err != nil {
		return runs.ModelCallCompleted{}, err
	}
	calibration, calibrated, err := calibrateModelContext(prepared, preparedFound, delta.InputTokens)
	if err != nil {
		return runs.ModelCallCompleted{}, err
	}
	consumption.ContextTokens = delta.InputTokens
	i.usageByProcess[processID] = nextUsage
	if preparedFound {
		delete(i.preparedContextByProcess, processID)
	}
	if calibrated {
		i.contextByProcess[processID] = calibration
	}
	return consumption, nil
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
