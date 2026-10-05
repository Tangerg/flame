package execution

import (
	"errors"
	"fmt"
	"sync"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

// requestTails carries each model call's Runtime-authored tail from the
// context reducer, which composes and measures it, to the model boundary,
// which sends it. The tail is a per-request projection of current Session
// state and the frozen deferred catalog: it follows the conversation so every
// earlier message stays a reusable cache prefix, and it never enters the
// context Scope adopts. One composition per call means the budget the reducer
// measured is exactly what the provider receives, even if that state changes
// in between.
type requestTails struct {
	mu        sync.Mutex
	byProcess map[agent.ProcessID]preparedTail
}

type preparedTail struct {
	effectID agent.EffectID
	sequence uint64
	messages []corechat.Message
}

func newRequestTails() requestTails {
	return requestTails{byProcess: make(map[agent.ProcessID]preparedTail)}
}

func (t *requestTails) prepare(invocation interaction.ModelInvocation, messages []corechat.Message) error {
	if !invocation.Valid() {
		return errors.New("execution: prepare request tail requires valid attribution")
	}
	processID := invocation.Relation().ProcessID()
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.byProcess[processID]; exists {
		return fmt.Errorf("execution: Process %s already has a prepared request tail", processID)
	}
	t.byProcess[processID] = preparedTail{
		effectID: invocation.EffectID(), sequence: invocation.ModelCallSequence(),
		messages: cloneChatMessages(messages),
	}
	return nil
}

// take releases the tail prepared for exactly this invocation. Every model
// call is reduced first, so a call without one is a broken invariant rather
// than a request to send nothing.
func (t *requestTails) take(invocation interaction.ModelInvocation) ([]corechat.Message, error) {
	if !invocation.Valid() {
		return nil, errors.New("execution: take request tail requires valid attribution")
	}
	processID := invocation.Relation().ProcessID()
	t.mu.Lock()
	defer t.mu.Unlock()
	prepared, found := t.byProcess[processID]
	if !found || prepared.effectID != invocation.EffectID() || prepared.sequence != invocation.ModelCallSequence() {
		return nil, fmt.Errorf("execution: model call %d of Process %s has no prepared request tail", invocation.ModelCallSequence(), processID)
	}
	delete(t.byProcess, processID)
	return prepared.messages, nil
}

// discard releases a tail whose call exits before taking it. Identity is
// matched under the lock so a stale exit cannot discard a later call's tail.
func (t *requestTails) discard(invocation interaction.ModelInvocation) {
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
