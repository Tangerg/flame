package execution

import (
	"errors"
	"sync"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Effect failures retain the first local cause before Scope classifies an
// unsettled dispatch. They describe diagnostics, never proof of an outcome.
type interactionEffectFailures struct {
	mu     sync.Mutex
	causes map[agent.EffectID]effectObservation
}

func (i *interactionEffectFailures) record(id agent.EffectID, cause error) {
	if cause == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.causes == nil {
		i.causes = make(map[agent.EffectID]effectObservation)
	}
	if _, exists := i.causes[id]; !exists {
		observation := effectObservation{ID: id.String(), Detail: executorDiagnostic(cause)}
		if evidence, found := errors.AsType[*toolcontract.CallError](cause); found && evidence.Validate() == nil {
			observation.Output = new(evidence.Evidence())
		}
		i.causes[id] = observation
	}
}

func (i *interactionEffectFailures) observations(ids []agent.EffectID) []effectObservation {
	i.mu.Lock()
	defer i.mu.Unlock()
	effects := make([]effectObservation, len(ids))
	for index, id := range ids {
		observation := i.causes[id]
		observation.ID = id.String()
		if observation.Output != nil {
			observation.Output = new(observation.Output.Clone())
		}
		effects[index] = observation
	}
	return effects
}

type effectObservation struct {
	ID     string
	Detail string
	Output *chat.ToolOutput
}
