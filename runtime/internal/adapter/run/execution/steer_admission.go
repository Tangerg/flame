package execution

import (
	"errors"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
)

func (i *interactionSession) admittedSteers(tree agent.TreeSnapshot) ([]runs.CommandAcceptance, error) {
	i.state.mu.Lock()
	pending := make(map[agent.SignalID]runs.PreparedSteer, len(i.state.pendingSteers))
	for id, value := range i.state.pendingSteers {
		pending[id] = value.Clone()
	}
	i.state.mu.Unlock()
	var accepted []runs.CommandAcceptance
	for _, snapshot := range tree.ProcessSnapshots() {
		if snapshot.Relation().ProcessID() != tree.RootID() {
			continue
		}
		for _, receipt := range snapshot.SignalReceipts() {
			steer, found := pending[receipt.ID()]
			if !found || steer.Acceptance == nil {
				continue
			}
			message, err := runs.MaterializeUserMessage(steer.Input)
			if err != nil {
				return nil, err
			}
			signal, err := interaction.NewSteerSignal(receipt.ID(), message)
			if err != nil {
				return nil, err
			}
			if !receipt.Matches(signal) {
				return nil, errors.New("execution: steer identity conflicts with its admission receipt")
			}
			accepted = append(accepted, steer.Acceptance.Clone())
		}
	}
	slices.SortFunc(accepted, func(a, b runs.CommandAcceptance) int { return strings.Compare(a.Key, b.Key) })
	return accepted, nil
}
