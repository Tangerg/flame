package execution

import (
	"errors"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	scoperun "github.com/Tangerg/scope/agent/run"
)

// Acceptance commits with its original SignalAdmitted batch. Recovery's
// Activated record cannot publish an already acknowledged command again.
func (i *interactionSession) admittedSteers(root scoperun.ProcessID, records []scoperun.Record) ([]runs.CommandAcceptance, error) {
	i.state.mu.Lock()
	pending := make(map[scoperun.SignalID]runs.PreparedSteer, len(i.state.pendingSteers))
	for id, value := range i.state.pendingSteers {
		pending[id] = value.Clone()
	}
	i.state.mu.Unlock()
	var accepted []runs.CommandAcceptance
	for _, record := range records {
		admission, ok := record.(scoperun.SignalAdmitted)
		if !ok || admission.Process != root {
			continue
		}
		steer, found := pending[admission.Signal.ID]
		if !found || steer.Acceptance == nil {
			continue
		}
		message, err := runs.MaterializeUserMessage(steer.Input)
		if err != nil {
			return nil, err
		}
		signal, err := materializeSteerSignal(admission.Signal.ID, message)
		if err != nil {
			return nil, err
		}
		if !sameSignal(admission.Signal, signal) {
			return nil, errors.New("execution: steer differs from its admission")
		}
		accepted = append(accepted, steer.Acceptance.Clone())
	}
	slices.SortFunc(accepted, func(a, b runs.CommandAcceptance) int { return strings.Compare(a.Key, b.Key) })
	return accepted, nil
}
