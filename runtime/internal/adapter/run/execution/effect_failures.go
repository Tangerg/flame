package execution

import (
	"errors"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/domain/run"

	scoperun "github.com/Tangerg/scope/agent/run"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type interactionEffectFailures struct {
	mu     sync.Mutex
	causes map[scoperun.EffectID]effectObservation
}

func (i *interactionEffectFailures) record(id scoperun.EffectID, cause error) {
	if cause == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.causes == nil {
		i.causes = make(map[scoperun.EffectID]effectObservation)
	}
	if _, exists := i.causes[id]; !exists {
		observation := effectObservation{Detail: run.Diagnostic(cause)}
		if evidence, found := errors.AsType[*toolcontract.CallError](cause); found && evidence.Validate() == nil {
			observation.Output = new(evidence.Evidence())
		}
		i.causes[id] = observation
	}
}

func (i *interactionEffectFailures) observation(id scoperun.EffectID) effectObservation {
	i.mu.Lock()
	defer i.mu.Unlock()
	observation := i.causes[id]
	if observation.Output != nil {
		observation.Output = new(observation.Output.Clone())
	}
	return observation
}

type effectObservation struct {
	Detail string
	Output *toolcontract.Output
}
