package execution

import (
	"bytes"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
)

type requestMaterial struct {
	mu           sync.Mutex
	byProcess    map[scoperun.ProcessID]preparedRequestMaterial
	calibrations map[scoperun.ProcessID]ModelContextTokenCalibration
}

type preparedRequestMaterial struct {
	effectID        scoperun.EffectID
	sequence        uint64
	request         *corechat.Request
	estimatedTokens int
}

type preparedRequestWire struct {
	Reference       preparedModelReference `json:"reference"`
	Request         *corechat.Request      `json:"request"`
	EstimatedTokens int                    `json:"estimatedTokens"`
}

func newRequestMaterial() requestMaterial {
	return requestMaterial{
		byProcess:    make(map[scoperun.ProcessID]preparedRequestMaterial),
		calibrations: make(map[scoperun.ProcessID]ModelContextTokenCalibration),
	}
}

func (t *requestMaterial) prepare(source modelCallSource, request *corechat.Request, estimatedTokens int) error {
	if !source.valid() || request == nil {
		return errors.New("execution: prepare request material requires valid attribution")
	}
	if estimatedTokens < 0 {
		return errors.New("execution: prepared request token estimate is negative")
	}
	request = request.Clone()
	request.Messages = providerMessages(request.Messages)
	t.mu.Lock()
	defer t.mu.Unlock()
	if prior, exists := t.byProcess[source.process]; exists {
		left, leftErr := json.Marshal(prior.request, json.Deterministic(true))
		right, rightErr := json.Marshal(request, json.Deterministic(true))
		if prior.effectID != source.effect || prior.sequence != source.sequence || prior.estimatedTokens != estimatedTokens || leftErr != nil || rightErr != nil || !bytes.Equal(left, right) {
			return fmt.Errorf("execution: Process %s already has different prepared request material", source.process)
		}
		return nil
	}
	t.byProcess[source.process] = preparedRequestMaterial{
		effectID: source.effect, sequence: source.sequence, request: request, estimatedTokens: estimatedTokens,
	}
	return nil
}

// Reading never retires a request: explicit replay still needs the original
// prepared input.
func (t *requestMaterial) providerRequest(source modelCallSource) (*corechat.Request, error) {
	if !source.valid() {
		return nil, errors.New("execution: model request requires valid attribution")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	prepared, found := t.byProcess[source.process]
	if !found || prepared.sequence != source.sequence {
		return nil, fmt.Errorf("execution: model call %d of Process %s has no prepared input", source.sequence, source.process)
	}
	return prepared.request.Clone(), nil
}

func (t *requestMaterial) observeInput(source modelCallSource, reportedTokens int64) error {
	if reportedTokens <= 0 {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	prepared, found := t.byProcess[source.process]
	if !found || prepared.sequence != source.sequence {
		return errors.New("execution: reported model input has no matching prepared request")
	}
	if prepared.estimatedTokens == 0 {
		return nil
	}
	calibration, err := NewModelContextTokenCalibration(reportedTokens, prepared.estimatedTokens)
	if err != nil {
		return err
	}
	t.calibrations[source.process] = calibration
	return nil
}

func (t *requestMaterial) calibration(process scoperun.ProcessID) ModelContextTokenCalibration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calibrations[process]
}

func (t *requestMaterial) checkpointCalibrations() map[scoperun.ProcessID]ModelContextTokenCalibration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return maps.Clone(t.calibrations)
}

func (t *requestMaterial) restoreCalibrations(values map[scoperun.ProcessID]ModelContextTokenCalibration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	clear(t.calibrations)
	maps.Copy(t.calibrations, values)
}

func (t *requestMaterial) snapshot(history scoperun.Log) ([]preparedRequestWire, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	retired, err := settledPreparations(history, t.byProcess)
	if err != nil {
		return nil, err
	}
	var result []preparedRequestWire
	for process, prepared := range t.byProcess {
		if _, complete := retired[prepared.effectID]; complete {
			continue
		}
		result = append(result, preparedRequestWire{
			Reference: preparedModelReference{Process: process, Effect: prepared.effectID, Sequence: prepared.sequence},
			Request:   prepared.request.Clone(), EstimatedTokens: prepared.estimatedTokens,
		})
	}
	slices.SortFunc(result, func(left, right preparedRequestWire) int {
		return strings.Compare(left.Reference.Process.String(), right.Reference.Process.String())
	})
	return result, nil
}

func (t *requestMaterial) acknowledge(history scoperun.Log) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	retired, err := settledPreparations(history, t.byProcess)
	if err != nil {
		return err
	}
	for process, prepared := range t.byProcess {
		if _, complete := retired[prepared.effectID]; complete {
			delete(t.byProcess, process)
		}
	}
	return nil
}

func (t *requestMaterial) restore(records []preparedRequestWire) error {
	prepared := make(map[scoperun.ProcessID]preparedRequestMaterial, len(records))
	for _, record := range records {
		ref := record.Reference
		if !ref.valid() || record.Request == nil || record.Request.Validate() != nil || record.EstimatedTokens < 0 {
			return errors.New("execution: invalid persisted provider request")
		}
		if _, exists := prepared[ref.Process]; exists {
			return errors.New("execution: persisted provider request repeats one Process")
		}
		prepared[ref.Process] = preparedRequestMaterial{
			effectID: ref.Effect, sequence: ref.Sequence, request: record.Request.Clone(), estimatedTokens: record.EstimatedTokens,
		}
	}
	t.mu.Lock()
	t.byProcess = prepared
	t.mu.Unlock()
	return nil
}

func collectPreparedModels(record scoperun.Stepped, preparation preparedRequestMaterial, models map[scoperun.EffectID]preparedModelReference) error {
	for _, declared := range record.Intents() {
		if !preparation.effectID.Precedes(declared.ID) {
			continue
		}
		intent, model, err := interaction.ModelCallFromEffect(declared.Effect)
		if err != nil {
			return err
		}
		if !model {
			continue
		}
		reference, _, err := modelPreparationReference(intent.Request.Messages)
		if err != nil {
			return err
		}
		if reference.Sequence != intent.Sequence {
			return errors.New("execution: settled model differs from its preparation sequence")
		}
		models[declared.ID] = reference
	}
	return nil
}

func settledPreparations(history scoperun.Log, prepared map[scoperun.ProcessID]preparedRequestMaterial) (map[scoperun.EffectID]struct{}, error) {
	if len(prepared) == 0 {
		return nil, nil
	}
	models := make(map[scoperun.EffectID]preparedModelReference)
	settled := make(map[scoperun.EffectID]struct{})
	for _, entry := range history.Entries {
		switch record := entry.Record.(type) {
		case scoperun.Stepped:
			preparation, owned := prepared[record.Process]
			if !owned {
				continue
			}
			if err := collectPreparedModels(record, preparation, models); err != nil {
				return nil, err
			}
		case scoperun.EffectSettled:
			if ref, model := models[record.ID]; model && record.Outcome.Status() != scoperun.SettlementStatusUnknown {
				settled[ref.Effect] = struct{}{}
			}
		}
	}
	return settled, nil
}
