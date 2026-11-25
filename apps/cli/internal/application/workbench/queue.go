package workbench

import (
	"errors"
	"slices"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
)

var ErrDispatchCanceling = errors.New("queued prompt is awaiting cancellation")

var errEntryDispatching = errors.New("prompt queue: entry is being dispatched")

type Queue struct {
	store     *Store
	bySession map[string][]PendingRun
}

func (q *Queue) Head(sessionID string) (PendingRun, bool) {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()
	pending := q.bySession[sessionID]
	if len(pending) == 0 {
		return PendingRun{}, false
	}
	return pending[0].clone(), true
}

func (q *Queue) EnqueueCommand(commandID authoring.CommandID, sessionID string, message authoring.Message, options authoring.RunOptions) (authoring.QueueEntry, error) {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()
	pending := PendingRun{
		EntryID: authoring.QueueEntryID(commandID),
		Command: authoring.StartRun{
			CommandID: commandID, SessionID: sessionID, Message: message.Clone(), Options: options.Clone(),
		},
	}
	next := append(slices.Clone(q.bySession[sessionID]), pending)
	if err := validatePendingRunSequence(sessionID, next); err != nil {
		return authoring.QueueEntry{}, err
	}
	if err := q.store.saveSessionState(sessionID, authoring.Message{}, next); err != nil {
		return authoring.QueueEntry{}, err
	}
	return pending.entry(), nil
}

func (q *Queue) Snapshot(sessionID string) authoring.QueueSnapshot {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()
	pending := q.bySession[sessionID]
	snapshot := authoring.QueueSnapshot{Entries: make([]authoring.QueueEntry, len(pending))}
	for index, entry := range pending {
		snapshot.Entries[index] = entry.entry()
	}
	if len(pending) != 0 && pending[0].dispatching() {
		snapshot.Dispatching = new(pending[0].EntryID)
	}
	return snapshot
}

func (q *Queue) BeginDispatch(sessionID string) (PendingRun, bool) {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()
	pending := q.bySession[sessionID]
	if len(pending) == 0 || pending[0].Dispatch != nil || pending[0].claim != unclaimedPrompt {
		return PendingRun{}, false
	}
	pending[0].claim = preparingPrompt
	return pending[0].clone(), true
}

func (q *Queue) Dispatching(sessionID string) (authoring.QueueEntry, bool) {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()
	pending := q.bySession[sessionID]
	if len(pending) == 0 || !pending[0].dispatching() {
		return authoring.QueueEntry{}, false
	}
	return pending[0].entry(), true
}

func (q *Queue) ReleasePreparation(sessionID string) {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()
	pending := q.bySession[sessionID]
	if len(pending) != 0 && pending[0].claim == preparingPrompt {
		pending[0].claim = unclaimedPrompt
	}
}

func (q *Queue) Hold(sessionID string, id authoring.QueueEntryID) error {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()
	pending := q.bySession[sessionID]
	index, err := waitingIndex(pending, id)
	if err != nil {
		return err
	}
	if pending[index].claim == editingPrompt {
		return errors.New("prompt queue: entry is already held")
	}
	pending[index].claim = editingPrompt
	return nil
}

func (q *Queue) Release(sessionID string, id authoring.QueueEntryID) error {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()
	pending := q.bySession[sessionID]
	index, err := waitingIndex(pending, id)
	if err != nil {
		return err
	}
	pending[index].claim = unclaimedPrompt
	return nil
}

func (q *Queue) Update(sessionID string, id authoring.QueueEntryID, message authoring.Message) error {
	return q.edit(sessionID, id, func(next []PendingRun, index int) ([]PendingRun, error) {
		next[index].Command.Message = message.Clone()
		next[index].Command.CommandID = mutation.NewCommandID()
		next[index].claim = unclaimedPrompt
		return next, nil
	})
}

func (q *Queue) Remove(sessionID string, id authoring.QueueEntryID) (authoring.QueueEntry, error) {
	var removed authoring.QueueEntry
	if err := q.edit(sessionID, id, func(next []PendingRun, index int) ([]PendingRun, error) {
		removed = next[index].entry()
		return slices.Delete(next, index, index+1), nil
	}); err != nil {
		return authoring.QueueEntry{}, err
	}
	return removed, nil
}

func (q *Queue) Move(sessionID string, id authoring.QueueEntryID, offset int) error {
	return q.edit(sessionID, id, func(next []PendingRun, index int) ([]PendingRun, error) {
		target := index + offset
		if offset == 0 || target < firstWaitingPrompt(next) || target >= len(next) {
			return nil, authoring.ErrMoveUnavailable
		}
		return movePrompt(next, index, target), nil
	})
}

func (q *Queue) Promote(sessionID string, id authoring.QueueEntryID) error {
	return q.edit(sessionID, id, func(next []PendingRun, index int) ([]PendingRun, error) {
		return movePrompt(next, index, firstWaitingPrompt(next)), nil
	})
}

func (q *Queue) edit(sessionID string, id authoring.QueueEntryID, change func([]PendingRun, int) ([]PendingRun, error)) error {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()
	current := q.bySession[sessionID]
	index, err := waitingIndex(current, id)
	if err != nil {
		return err
	}
	next, err := change(slices.Clone(current), index)
	if err != nil {
		return err
	}
	return q.save(sessionID, next)
}

func (q *Queue) save(sessionID string, pending []PendingRun) error {
	if err := validatePendingRunSequence(sessionID, pending); err != nil {
		return err
	}
	return q.store.saveSessionState(sessionID, q.store.drafts[sessionID], pending)
}

func (q *Queue) install(sessionID string, pending []PendingRun) {
	if len(pending) == 0 {
		delete(q.bySession, sessionID)
		return
	}
	q.bySession[sessionID] = clonePendingRunSlice(pending)
}

func waitingIndex(pending []PendingRun, id authoring.QueueEntryID) (int, error) {
	index := slices.IndexFunc(pending, func(entry PendingRun) bool { return entry.EntryID == id })
	if index < 0 {
		return -1, authoring.ErrEntryNotFound
	}
	if pending[index].dispatching() {
		return -1, errEntryDispatching
	}
	return index, nil
}

func firstWaitingPrompt(pending []PendingRun) int {
	if len(pending) != 0 && pending[0].dispatching() {
		return 1
	}
	return 0
}

func movePrompt(pending []PendingRun, from, to int) []PendingRun {
	entry := pending[from]
	pending = slices.Delete(pending, from, from+1)
	return slices.Insert(pending, to, entry)
}
