package agentexec

import (
	"sync"

	agent "github.com/Tangerg/scope/agent"
)

// Effect failures retain the first local cause before Scope classifies an
// unsettled dispatch. They describe diagnostics, never proof of an outcome.
type interactionEffectFailures struct {
	mu     sync.Mutex
	causes map[agent.EffectID]string
}

func (i *interactionEffectFailures) record(id agent.EffectID, cause error) {
	if cause == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.causes == nil {
		i.causes = make(map[agent.EffectID]string)
	}
	if _, exists := i.causes[id]; !exists {
		i.causes[id] = executorDiagnostic(cause)
	}
}

func (i *interactionEffectFailures) observations(ids []agent.EffectID) []effectObservation {
	i.mu.Lock()
	defer i.mu.Unlock()
	effects := make([]effectObservation, len(ids))
	for index, id := range ids {
		effects[index] = effectObservation{ID: id.String(), Detail: i.causes[id]}
	}
	return effects
}

type effectObservation struct {
	ID     string
	Detail string
}
