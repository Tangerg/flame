package workbench

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/queue"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
)

var ErrDispatchCanceling = errors.New("queued prompt is awaiting cancellation")

// Queue owns the transaction between the live FIFO and the durable authoring
// outbox. A failed write leaves the reservation and every editable entry intact;
// an acknowledged start releases its FIFO position only after durable settlement.
type Queue struct {
	mu    sync.Mutex
	store *Store
	fifo  *queue.Queue
}

func NewQueue(store *Store) (*Queue, error) {
	if store == nil {
		return nil, errors.New("prompt queue requires an open workbench")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return nil, errors.New("prompt queue requires an open workbench")
	}
	return &Queue{store: store, fifo: queue.New()}, nil
}

// EnqueueCommand transfers a prompt out of its draft and into both queue
// representations before a caller may start Runtime delivery.
func (q *Queue) EnqueueCommand(commandID replay.CommandID, sessionID string, message prompt.Message, options prompt.RunOptions) (queue.Entry, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	before := q.fifo.State(sessionID)
	entry, err := q.fifo.EnqueueCommand(commandID, sessionID, message, options)
	if err != nil {
		return queue.Entry{}, err
	}
	if err := q.store.StagePendingRun(PendingRun{
		State: PendingRunQueued, Replay: replay.UnprotectedGuard(),
		CancelReplay: replay.UnprotectedGuard(),
		Command:      prompt.StartRun{CommandID: commandID, SessionID: sessionID, Message: message.Clone(), Options: options.Clone()},
	}); err != nil {
		return queue.Entry{}, q.rollback(sessionID, before, err)
	}
	return entry, nil
}

func (q *Queue) Snapshot(sessionID string) queue.Snapshot {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.fifo.Snapshot(sessionID)
}

func (q *Queue) State(sessionID string) queue.State {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.fifo.State(sessionID)
}

func (q *Queue) BeginDispatch(sessionID string) (queue.Entry, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.fifo.BeginDispatch(sessionID)
}

func (q *Queue) Dispatching(sessionID string) (queue.Entry, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.fifo.Dispatching(sessionID)
}

func (q *Queue) ReleaseDispatch(sessionID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.fifo.ReleaseDispatch(sessionID)
}

func (q *Queue) Hold(sessionID string, id queue.EntryID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.fifo.Hold(sessionID, id)
}

func (q *Queue) Release(sessionID string, id queue.EntryID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.fifo.Release(sessionID, id)
}

func (q *Queue) Update(sessionID string, id queue.EntryID, message prompt.Message) error {
	return q.commit(sessionID, func() error {
		if err := q.fifo.Update(sessionID, id, message, mutation.NewCommandID()); err != nil {
			return err
		}
		return q.fifo.Release(sessionID, id)
	})
}

func (q *Queue) Remove(sessionID string, id queue.EntryID) (queue.Entry, error) {
	var removed queue.Entry
	err := q.commit(sessionID, func() error {
		var err error
		removed, err = q.fifo.Remove(sessionID, id)
		return err
	})
	if err != nil {
		return queue.Entry{}, err
	}
	return removed, nil
}

func (q *Queue) Move(sessionID string, id queue.EntryID, offset int) error {
	return q.commit(sessionID, func() error { return q.fifo.Move(sessionID, id, offset) })
}

func (q *Queue) Promote(sessionID string, id queue.EntryID) error {
	return q.commit(sessionID, func() error { return q.fifo.Promote(sessionID, id) })
}

// Restore installs the outbox owned by this workbench, including the exact
// opening reservation. A caller cannot substitute another queue projection.
func (q *Queue) Restore(sessionID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	pending := q.store.PendingRuns(sessionID)
	commands := make([]prompt.StartRun, len(pending))
	for index, entry := range pending {
		commands[index] = entry.Command.Clone()
	}
	var dispatching replay.CommandID
	if len(pending) > 0 && pending[0].State != PendingRunQueued {
		dispatching = pending[0].Command.CommandID
	}
	return q.fifo.Restore(sessionID, commands, dispatching)
}

// ForgetSession releases a projection after its durable authoring state has
// been retired by the Session workflow.
func (q *Queue) ForgetSession(sessionID string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.fifo.Clear(sessionID)
}

func (q *Queue) DispatchCanceling(sessionID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	entry, ok := q.fifo.Dispatching(sessionID)
	if !ok {
		return false
	}
	pending, found := q.pendingRun(sessionID, entry.CommandID)
	return found && pending.State == PendingRunCanceling
}

func (q *Queue) SettleDispatch(sessionID string) (queue.Entry, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	entry, ok := q.fifo.Dispatching(sessionID)
	if !ok {
		return queue.Entry{}, false, nil
	}
	if pending, found := q.pendingRun(sessionID, entry.CommandID); found && pending.State == PendingRunCanceling {
		return queue.Entry{}, false, ErrDispatchCanceling
	}
	return q.retire(sessionID, entry.CommandID)
}

// Retire closes the exact accepted, recovered or canceled opening. History and
// outbox ownership commit before releasing its reservation, including retries
// after history alone committed during an earlier failed settlement.
func (q *Queue) Retire(sessionID string, commandID replay.CommandID) (queue.Entry, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.retire(sessionID, commandID)
}

func (q *Queue) retire(sessionID string, commandID replay.CommandID) (queue.Entry, bool, error) {
	entry, dispatching := q.fifo.Dispatching(sessionID)
	_, pending := q.pendingRun(sessionID, commandID)
	if !dispatching {
		present := slices.ContainsFunc(q.fifo.Snapshot(sessionID).Entries, func(entry queue.Entry) bool { return entry.CommandID == commandID })
		if !present && !pending {
			return queue.Entry{}, false, nil
		}
		return queue.Entry{}, false, errors.New("dispatching prompt command ownership was released before settlement")
	}
	if entry.CommandID != commandID {
		return queue.Entry{}, false, errors.New("dispatching prompt command identity changed")
	}
	if pending {
		if err := q.store.AcknowledgePendingRun(sessionID, commandID); err != nil {
			return queue.Entry{}, false, err
		}
	}
	retired, err := q.fifo.RetireCommand(sessionID, commandID)
	return retired, err == nil, err
}

// RequeueDispatch is valid only after Runtime definitively refused this exact
// command. The durable replacement owns the new identity before FIFO release.
func (q *Queue) RequeueDispatch(sessionID string, commandID replay.CommandID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	entry, ok := q.fifo.Dispatching(sessionID)
	if !ok || entry.CommandID != commandID {
		return errors.New("dispatching prompt command identity changed")
	}
	replacement, err := q.store.RequeuePendingRun(sessionID, commandID)
	if err != nil {
		return err
	}
	return q.fifo.RequeueDispatch(sessionID, commandID, replacement)
}

func (q *Queue) pendingRun(sessionID string, commandID replay.CommandID) (PendingRun, bool) {
	for _, pending := range q.store.PendingRuns(sessionID) {
		if pending.Command.CommandID == commandID {
			return pending, true
		}
	}
	return PendingRun{}, false
}

func (q *Queue) commit(sessionID string, mutate func() error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	before := q.fifo.State(sessionID)
	if err := mutate(); err != nil {
		return q.rollback(sessionID, before, err)
	}
	if err := q.persist(sessionID); err != nil {
		return q.rollback(sessionID, before, err)
	}
	return nil
}

func (q *Queue) persist(sessionID string) error {
	state := q.fifo.State(sessionID)
	commands := make([]PendingRun, 0, len(state.Entries))
	dispatchingID, hasDispatch := state.DispatchingID()
	for _, entry := range state.Entries {
		if hasDispatch && entry.ID == dispatchingID {
			if pending, found := q.pendingRun(sessionID, entry.CommandID); found {
				commands = append(commands, pending)
			}
			continue
		}
		commands = append(commands, PendingRun{
			State: PendingRunQueued, Replay: replay.UnprotectedGuard(), CancelReplay: replay.UnprotectedGuard(),
			Command: prompt.StartRun{CommandID: entry.CommandID, SessionID: entry.SessionID, Message: entry.Message.Clone(), Options: entry.Options.Clone()},
		})
	}
	return q.store.SavePendingRuns(sessionID, commands)
}

func (q *Queue) rollback(sessionID string, before queue.State, cause error) error {
	if err := q.fifo.RestoreState(sessionID, before); err != nil {
		return errors.Join(cause, fmt.Errorf("restore prompt queue: %w", err))
	}
	return cause
}
