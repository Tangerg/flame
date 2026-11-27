package workbench

import (
	"errors"
	"slices"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
)

func (q *Queue) PrepareDispatch(sessionID string, commandID authoring.CommandID, replay authoring.ReplayGuard, input *PreparedInput) (PendingRun, error) {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()
	current, err := q.current(sessionID, commandID)
	if err != nil {
		return PendingRun{}, err
	}
	if current.Dispatch != nil {
		_, err := current.ReplayCommand()
		return current.clone(), err
	}
	blocks, digest, err := input.bind(q.store, current.Command.Message)
	if err != nil {
		return PendingRun{}, err
	}
	prepared, err := current.prepare(replay, blocks, digest)
	if err != nil {
		return PendingRun{}, err
	}
	if err := q.replaceHead(sessionID, prepared); err != nil {
		return PendingRun{}, err
	}
	return prepared.clone(), nil
}

func (q *Queue) StageCancellation(sessionID string, commandID authoring.CommandID, replay authoring.ReplayGuard) (PendingRun, error) {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()
	current, err := q.current(sessionID, commandID)
	if err != nil {
		return PendingRun{}, err
	}
	if current.Cancellation() != nil {
		return current.clone(), nil
	}
	canceling, err := current.beginCancellation(replay)
	if err != nil {
		return PendingRun{}, err
	}
	if err := q.replaceHead(sessionID, canceling); err != nil {
		return PendingRun{}, err
	}
	return canceling.clone(), nil
}

func (q *Queue) DispatchCanceling(sessionID string) bool {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()
	pending := q.bySession[sessionID]
	return len(pending) != 0 && pending[0].Cancellation() != nil
}

func (q *Queue) SettleDispatch(sessionID string, commandID authoring.CommandID) (authoring.QueueEntry, bool, error) {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()
	pending := q.bySession[sessionID]
	if len(pending) != 0 && pending[0].Command.CommandID == commandID && pending[0].Cancellation() != nil {
		return authoring.QueueEntry{}, false, ErrDispatchCanceling
	}
	return q.retire(sessionID, commandID)
}

func (q *Queue) Retire(sessionID string, commandID authoring.CommandID) (authoring.QueueEntry, bool, error) {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()
	return q.retire(sessionID, commandID)
}

func (q *Queue) retire(sessionID string, commandID authoring.CommandID) (authoring.QueueEntry, bool, error) {
	pending := q.bySession[sessionID]
	index := pendingRunIndex(pending, commandID)
	if index < 0 {
		return authoring.QueueEntry{}, false, nil
	}
	if index != 0 || pending[0].Dispatch == nil {
		return authoring.QueueEntry{}, false, errors.New("only the dispatched queue head can be acknowledged")
	}
	entry := pending[0].entry()
	update := q.store.sessionUpdate(sessionID)
	update.runs = pending[1:]
	if err := q.store.recordPromptSettlement(update, entry.Message); err != nil {
		return authoring.QueueEntry{}, false, err
	}
	return entry, true, nil
}

func (q *Queue) RequeueDispatch(sessionID string, commandID authoring.CommandID) error {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()
	current, err := q.current(sessionID, commandID)
	if err != nil {
		return err
	}
	next, err := current.requeue()
	if err != nil {
		return err
	}
	return q.replaceHead(sessionID, next)
}

func (q *Queue) current(sessionID string, commandID authoring.CommandID) (PendingRun, error) {
	pending := q.bySession[sessionID]
	if len(pending) == 0 || pending[0].Command.CommandID != commandID {
		return PendingRun{}, errors.New("pending run no longer owns the queue head")
	}
	return pending[0], nil
}

func (q *Queue) replaceHead(sessionID string, entry PendingRun) error {
	next := slices.Clone(q.bySession[sessionID])
	next[0] = entry
	return q.save(sessionID, next)
}
