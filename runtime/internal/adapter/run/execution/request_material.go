package execution

import (
	"errors"
	"fmt"
	"sync"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

// requestMaterial transfers one complete admitted provider request from reduction
// to Call/Stream. Its contributed/transformed projection never enters Scope's
// retained conversation or Options.
type requestMaterial struct {
	mu        sync.Mutex
	byProcess map[agent.ProcessID]preparedRequestMaterial
}

type preparedRequestMaterial struct {
	effectID agent.EffectID
	sequence uint64
	request  *corechat.Request
}

func newRequestMaterial() requestMaterial {
	return requestMaterial{byProcess: make(map[agent.ProcessID]preparedRequestMaterial)}
}

func (t *requestMaterial) prepare(invocation interaction.ModelInvocation, request *corechat.Request) error {
	if !invocation.Valid() || request == nil {
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
		request: request.Clone(),
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
