package execution

import (
	"errors"
	"fmt"
	"sync"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

// requestMaterial transfers the complete ephemeral context measured by the
// reducer to the provider boundary. Neither contributed instructions nor the
// current Session tail enters Scope's WorkingContext.
type requestMaterial struct {
	mu        sync.Mutex
	byProcess map[agent.ProcessID]preparedRequestMaterial
}

type preparedRequestMaterial struct {
	effectID agent.EffectID
	sequence uint64
	prefixAt int
	prefix   []corechat.Message
	trailer  []corechat.Message
}

func newRequestMaterial() requestMaterial {
	return requestMaterial{byProcess: make(map[agent.ProcessID]preparedRequestMaterial)}
}

func (t *requestMaterial) prepare(invocation interaction.ModelInvocation, prefixAt int, prefix, trailer []corechat.Message) error {
	if !invocation.Valid() || prefixAt < 0 {
		return errors.New("execution: prepare request material requires valid attribution")
	}
	processID := invocation.Relation().ProcessID()
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.byProcess[processID]; exists {
		return fmt.Errorf("execution: Process %s already has a prepared request material", processID)
	}
	t.byProcess[processID] = preparedRequestMaterial{
		effectID: invocation.EffectID(), sequence: invocation.ModelCallSequence(),
		prefixAt: prefixAt, prefix: cloneChatMessages(prefix), trailer: cloneChatMessages(trailer),
	}
	return nil
}

// take releases the material prepared for exactly this invocation. Every model
// call is reduced first, so a call without one is a broken invariant rather
// than a request to send nothing.
func (t *requestMaterial) take(invocation interaction.ModelInvocation) (preparedRequestMaterial, error) {
	if !invocation.Valid() {
		return preparedRequestMaterial{}, errors.New("execution: take request material requires valid attribution")
	}
	processID := invocation.Relation().ProcessID()
	t.mu.Lock()
	defer t.mu.Unlock()
	prepared, found := t.byProcess[processID]
	if !found || prepared.effectID != invocation.EffectID() || prepared.sequence != invocation.ModelCallSequence() {
		return preparedRequestMaterial{}, fmt.Errorf("execution: model call %d of Process %s has no prepared request material", invocation.ModelCallSequence(), processID)
	}
	delete(t.byProcess, processID)
	return prepared, nil
}

// discard releases material whose call exits before taking it. Identity is
// matched under the lock so a stale exit cannot discard a later call's material.
func (t *requestMaterial) discard(invocation interaction.ModelInvocation) {
	if !invocation.Valid() {
		return
	}
	processID := invocation.Relation().ProcessID()
	t.mu.Lock()
	defer t.mu.Unlock()
	prepared, found := t.byProcess[processID]
	if found && prepared.effectID == invocation.EffectID() && prepared.sequence == invocation.ModelCallSequence() {
		delete(t.byProcess, processID)
	}
}
