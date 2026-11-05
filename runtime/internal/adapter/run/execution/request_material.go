package execution

import (
	"bytes"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
)

// Prepared provider input is a Flame fact: contributed instructions and the
// Session tail are deliberately absent from Scope's retained conversation. It
// is persisted beside the log before the preparation Effect is acknowledged.
type requestMaterial struct {
	mu        sync.Mutex
	byProcess map[scoperun.ProcessID]preparedRequestMaterial
}

type preparedRequestMaterial struct {
	effectID scoperun.EffectID
	sequence uint64
	request  *corechat.Request
}

type preparedRequestWire struct {
	Reference       preparedModelReference `json:"reference"`
	Request         *corechat.Request      `json:"request"`
	EstimatedTokens int                    `json:"estimatedTokens"`
}

func newRequestMaterial() requestMaterial {
	return requestMaterial{byProcess: make(map[scoperun.ProcessID]preparedRequestMaterial)}
}

func (t *requestMaterial) prepare(source modelCallSource, request *corechat.Request) error {
	if !source.valid() || request == nil {
		return errors.New("execution: prepare request material requires valid attribution")
	}
	request = request.Clone()
	request.Messages = providerMessages(request.Messages)
	t.mu.Lock()
	defer t.mu.Unlock()
	if prior, exists := t.byProcess[source.process]; exists {
		left, leftErr := json.Marshal(prior.request, json.Deterministic(true))
		right, rightErr := json.Marshal(request, json.Deterministic(true))
		if prior.effectID != source.effect || prior.sequence != source.sequence || leftErr != nil || rightErr != nil || !bytes.Equal(left, right) {
			return fmt.Errorf("execution: Process %s already has different prepared request material", source.process)
		}
		return nil
	}
	t.byProcess[source.process] = preparedRequestMaterial{effectID: source.effect, sequence: source.sequence, request: request}
	return nil
}

// Reading never retires a request. A crash after dispatch can leave the model
// effect Unknown, and explicit replay still needs the original prepared input.
func (t *requestMaterial) take(source modelCallSource) (preparedRequestMaterial, error) {
	if !source.valid() {
		return preparedRequestMaterial{}, errors.New("execution: model request requires valid attribution")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	prepared, found := t.byProcess[source.process]
	if !found || prepared.sequence != source.sequence {
		return preparedRequestMaterial{}, fmt.Errorf("execution: model call %d of Process %s has no prepared input", source.sequence, source.process)
	}
	prepared.request = prepared.request.Clone()
	return prepared, nil
}

func (t *requestMaterial) snapshot(history scoperun.Log, accounting *interactionAccounting) ([]preparedRequestWire, error) {
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
		accounting.mu.Lock()
		calibration := accounting.preparedContextByProcess[process]
		accounting.mu.Unlock()
		result = append(result, preparedRequestWire{
			Reference: preparedModelReference{Process: process, Effect: prepared.effectID, Sequence: prepared.sequence},
			Request:   prepared.request.Clone(), EstimatedTokens: calibration.estimated,
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

func (t *requestMaterial) restore(records []preparedRequestWire, accounting *interactionAccounting) error {
	prepared := make(map[scoperun.ProcessID]preparedRequestMaterial, len(records))
	calibration := make(map[scoperun.ProcessID]preparedModelContext, len(records))
	for _, record := range records {
		ref := record.Reference
		if !ref.valid() || record.Request == nil || record.Request.Validate() != nil || record.EstimatedTokens < 0 {
			return errors.New("execution: invalid persisted provider request")
		}
		if _, exists := prepared[ref.Process]; exists {
			return errors.New("execution: persisted provider request repeats one Process")
		}
		prepared[ref.Process] = preparedRequestMaterial{effectID: ref.Effect, sequence: ref.Sequence, request: record.Request.Clone()}
		if record.EstimatedTokens > 0 {
			calibration[ref.Process] = preparedModelContext{effectID: ref.Effect, sequence: ref.Sequence, estimated: record.EstimatedTokens}
		}
	}
	t.mu.Lock()
	t.byProcess = prepared
	t.mu.Unlock()
	accounting.mu.Lock()
	accounting.preparedContextByProcess = calibration
	accounting.mu.Unlock()
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
		case scoperun.EffectIntent:
			preparation, owned := prepared[record.ID.Process]
			if !owned || !preparation.effectID.Precedes(record.ID) {
				continue
			}
			request, model, err := interaction.ModelRequestFromEffect(record.Effect)
			if err != nil {
				return nil, err
			}
			if model {
				reference, _, err := modelPreparationReference(request.Messages)
				if err != nil {
					return nil, err
				}
				models[record.ID] = reference
			}
		case scoperun.EffectSettled:
			if ref, model := models[record.ID]; model && record.Outcome.Status() != scoperun.SettlementStatusUnknown {
				settled[ref.Effect] = struct{}{}
			}
		}
	}
	return settled, nil
}
