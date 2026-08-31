// Package promptqueue owns follow-up messages waiting behind a running turn.
// It is application state rather than terminal state: views receive detached
// snapshots, while runtime adapters continue to see ordinary StartRun calls.
package promptqueue

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	cliidentity "github.com/Tangerg/flame/cli/internal/domain/identity"
)

var (
	ErrEntryNotFound     = errors.New("prompt queue: entry not found")
	ErrEntryHeld         = errors.New("prompt queue: entry is already held")
	ErrEntryDispatching  = errors.New("prompt queue: entry is being dispatched")
	ErrMoveUnavailable   = errors.New("prompt queue: move unavailable")
	ErrSessionIDRequired = errors.New("prompt queue: session id is empty")
)

// Entry is one immutable queue projection. Message owns a detached attachment
// slice, so callers cannot mutate the queue through a snapshot.
type Entry struct {
	ID        EntryID
	CommandID agent.CommandID
	SessionID string
	Message   agent.Message
	Options   agent.RunOptions
	Held      bool
}

// Snapshot is one session's detached FIFO view.
type Snapshot struct {
	Entries []Entry
}

// State is a transactional queue snapshot. Dispatching uses pointer presence
// because a reservation is optional and EntryID has no sentinel value.
type State struct {
	Entries     []Entry
	Dispatching *EntryID
}

func (s State) DispatchingID() (EntryID, bool) {
	if s.Dispatching == nil {
		return EntryID{}, false
	}
	return *s.Dispatching, true
}

// Queue keeps independent FIFO sequences for every session visited by the app.
// This prevents a failed follow-up from moving to another session when the
// user switches away and later comes back.
type Queue struct {
	mu          sync.RWMutex
	nextID      uint64
	entries     map[string][]Entry
	dispatching map[string]EntryID
}

func New() *Queue {
	return &Queue{
		entries:     make(map[string][]Entry),
		dispatching: make(map[string]EntryID),
	}
}

func (q *Queue) Enqueue(sessionID string, message agent.Message) (Entry, error) {
	commandID, err := agent.NewCommandID()
	if err != nil {
		return Entry{}, fmt.Errorf("prompt queue: %w", err)
	}
	return q.EnqueueCommand(commandID, sessionID, message, agent.RunOptions{Limits: agent.UnlimitedRunLimits()})
}

// EnqueueCommand preserves a mutation identity already allocated by the
// authoring transaction. Queue edits allocate a new identity because changing
// content creates a different runtime operation fingerprint.
func (q *Queue) EnqueueCommand(commandID agent.CommandID, sessionID string, message agent.Message, options agent.RunOptions) (Entry, error) {
	if err := validateEntry(commandID, sessionID, message, options); err != nil {
		return Entry{}, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ensureStorage()
	ids, err := q.allocateEntryIDs(1)
	if err != nil {
		return Entry{}, err
	}
	entry := Entry{
		ID: ids[0], CommandID: commandID, SessionID: strings.Clone(sessionID),
		Message: message.Clone(), Options: options.Clone(),
	}
	q.entries[sessionID] = append(q.entries[sessionID], entry)
	return cloneEntry(entry), nil
}

// Restore replaces one session queue from a durable authoring outbox. Command
// identity and order are preserved so runtime retries remain idempotent. A
// non-empty dispatchingCommandID must identify the first command: it restores
// ownership of an interrupted runtime handshake without exposing that command
// to ordinary queue mutations.
func (q *Queue) Restore(sessionID string, commands []agent.StartRun, dispatchingCommandID agent.CommandID) error {
	entries := make([]Entry, len(commands))
	seen := make(map[agent.CommandID]struct{}, len(commands))
	for index, command := range commands {
		if command.SessionID != sessionID {
			return fmt.Errorf("prompt queue: command %d belongs to session %s", index+1, command.SessionID)
		}
		if err := validateEntry(command.CommandID, sessionID, command.Message, command.Options); err != nil {
			return err
		}
		if _, duplicate := seen[command.CommandID]; duplicate {
			return fmt.Errorf("prompt queue: command %s is duplicated", command.CommandID)
		}
		seen[command.CommandID] = struct{}{}
		entries[index] = Entry{
			CommandID: command.CommandID, SessionID: sessionID,
			Message: command.Message.Clone(), Options: command.Options.Clone(),
		}
	}
	if dispatchingCommandID != "" {
		if err := dispatchingCommandID.Validate(); err != nil {
			return fmt.Errorf("prompt queue: dispatch command: %w", err)
		}
		if len(entries) == 0 || entries[0].CommandID != dispatchingCommandID {
			return errors.New("prompt queue: dispatch command is not the first entry")
		}
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ensureStorage()
	ids, err := q.allocateEntryIDs(len(entries))
	if err != nil {
		return err
	}
	for index := range entries {
		entries[index].ID = ids[index]
	}
	if len(entries) == 0 {
		delete(q.entries, sessionID)
	} else {
		q.entries[sessionID] = entries
	}
	if dispatchingCommandID == "" {
		delete(q.dispatching, sessionID)
	} else {
		q.dispatching[sessionID] = entries[0].ID
	}
	return nil
}

// RestoreState replaces one session with a previously observed transactional
// state. It is intended for rolling back an in-memory mutation when the
// corresponding durable authoring transaction cannot be committed.
func (q *Queue) RestoreState(sessionID string, state State) error {
	entries := make([]Entry, len(state.Entries))
	seenIDs := make(map[EntryID]struct{}, len(state.Entries))
	seenCommands := make(map[agent.CommandID]struct{}, len(state.Entries))
	var maximumID uint64
	for index, entry := range state.Entries {
		if entry.SessionID != sessionID {
			return fmt.Errorf("prompt queue: entry %d belongs to session %s", index+1, entry.SessionID)
		}
		if err := entry.ID.Validate(); err != nil {
			return fmt.Errorf("prompt queue: entry %d: %w", index+1, err)
		}
		if err := validateEntry(entry.CommandID, sessionID, entry.Message, entry.Options); err != nil {
			return err
		}
		if _, duplicate := seenIDs[entry.ID]; duplicate {
			return fmt.Errorf("prompt queue: entry %s is duplicated", entry.ID)
		}
		if _, duplicate := seenCommands[entry.CommandID]; duplicate {
			return fmt.Errorf("prompt queue: command %s is duplicated", entry.CommandID)
		}
		seenIDs[entry.ID] = struct{}{}
		seenCommands[entry.CommandID] = struct{}{}
		maximumID = max(maximumID, entry.ID.value)
		entries[index] = cloneEntry(entry)
	}
	if state.Dispatching != nil {
		if err := state.Dispatching.Validate(); err != nil {
			return fmt.Errorf("prompt queue: dispatch reservation: %w", err)
		}
		if len(entries) == 0 || entries[0].ID != *state.Dispatching {
			return errors.New("prompt queue: dispatch reservation is not the first entry")
		}
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	q.ensureStorage()
	q.nextID = max(q.nextID, maximumID)
	if len(entries) == 0 {
		delete(q.entries, sessionID)
	} else {
		q.entries[sessionID] = entries
	}
	if state.Dispatching == nil {
		delete(q.dispatching, sessionID)
	} else {
		q.dispatching[sessionID] = *state.Dispatching
	}
	return nil
}

func (q *Queue) Snapshot(sessionID string) Snapshot {
	q.mu.RLock()
	defer q.mu.RUnlock()
	entries := q.entries[sessionID]
	out := make([]Entry, len(entries))
	for index, entry := range entries {
		out[index] = cloneEntry(entry)
	}
	return Snapshot{Entries: out}
}

func (q *Queue) State(sessionID string) State {
	q.mu.RLock()
	defer q.mu.RUnlock()
	entries := q.entries[sessionID]
	out := make([]Entry, len(entries))
	for index, entry := range entries {
		out[index] = cloneEntry(entry)
	}
	state := State{Entries: out}
	if dispatching, exists := q.dispatching[sessionID]; exists {
		state.Dispatching = new(dispatching)
	}
	return state
}

// BeginDispatch reserves the first dispatchable entry. While the reservation
// is open, authoring mutations may reorder only the entries behind it. This
// keeps the runtime mutation identity independent from transient queue
// selection and prevents a priority edit from changing the command being
// acknowledged or canceled.
func (q *Queue) BeginDispatch(sessionID string) (Entry, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ensureStorage()
	entries := q.entries[sessionID]
	if _, dispatching := q.dispatching[sessionID]; len(entries) == 0 || entries[0].Held || dispatching {
		return Entry{}, false
	}
	q.dispatching[sessionID] = entries[0].ID
	return cloneEntry(entries[0]), true
}

// Dispatching returns the exact entry whose StartRun handshake is owned by the
// current process. The entry remains reserved until its first event commits the
// dispatch or delivery is released for recovery.
func (q *Queue) Dispatching(sessionID string) (Entry, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	id, reserved := q.dispatching[sessionID]
	index := entryIndex(q.entries[sessionID], id)
	if !reserved || index < 0 {
		return Entry{}, false
	}
	return cloneEntry(q.entries[sessionID][index]), true
}

// CommitDispatch removes only the reserved entry after its SegmentStarted
// event has been folded into the active conversation.
func (q *Queue) CommitDispatch(sessionID string) (Entry, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	id, reserved := q.dispatching[sessionID]
	index := entryIndex(q.entries[sessionID], id)
	if !reserved || index < 0 {
		return Entry{}, ErrEntryNotFound
	}
	entry := q.entries[sessionID][index]
	removed := cloneEntry(entry)
	q.removeAt(sessionID, index)
	delete(q.dispatching, sessionID)
	return removed, nil
}

// RetireCommand removes the exact command settled by runtime recovery. Unlike
// an authoring removal, it is allowed to close a dispatch reservation and is
// therefore safe both before and after a local opening handshake is released.
func (q *Queue) RetireCommand(sessionID string, commandID agent.CommandID) (Entry, error) {
	if err := commandID.Validate(); err != nil {
		return Entry{}, fmt.Errorf("prompt queue: %w", err)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	index := slices.IndexFunc(q.entries[sessionID], func(entry Entry) bool {
		return entry.CommandID == commandID
	})
	if index < 0 {
		return Entry{}, ErrEntryNotFound
	}
	removed := cloneEntry(q.entries[sessionID][index])
	if dispatching, exists := q.dispatching[sessionID]; exists && dispatching == removed.ID {
		delete(q.dispatching, sessionID)
	}
	q.removeAt(sessionID, index)
	return removed, nil
}

// ReleaseDispatch returns the reserved entry to ordinary FIFO ownership.
func (q *Queue) ReleaseDispatch(sessionID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, exists := q.dispatching[sessionID]; !exists {
		return false
	}
	delete(q.dispatching, sessionID)
	return true
}

// Hold prevents the FIFO consumer from dispatching an entry while an editor
// owns it. A held entry remains visible and keeps its position in the queue.
func (q *Queue) Hold(sessionID string, id EntryID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := q.entries[sessionID]
	index := entryIndex(entries, id)
	if index < 0 {
		return ErrEntryNotFound
	}
	if dispatching, exists := q.dispatching[sessionID]; exists && dispatching == id {
		return ErrEntryDispatching
	}
	if entries[index].Held {
		return ErrEntryHeld
	}
	entries[index].Held = true
	q.entries[sessionID] = entries
	return nil
}

// Release makes a held entry dispatchable again. It is idempotent so dialog
// teardown can safely release ownership after any close path.
func (q *Queue) Release(sessionID string, id EntryID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := q.entries[sessionID]
	index := entryIndex(entries, id)
	if index < 0 {
		return ErrEntryNotFound
	}
	if dispatching, exists := q.dispatching[sessionID]; exists && dispatching == id {
		return ErrEntryDispatching
	}
	if !entries[index].Held {
		return nil
	}
	entries[index].Held = false
	q.entries[sessionID] = entries
	return nil
}

func (q *Queue) Update(sessionID string, id EntryID, message agent.Message) error {
	commandID, err := agent.NewCommandID()
	if err != nil {
		return fmt.Errorf("prompt queue: %w", err)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	index := entryIndex(q.entries[sessionID], id)
	if index < 0 {
		return ErrEntryNotFound
	}
	if err := validateEntry(commandID, sessionID, message, q.entries[sessionID][index].Options); err != nil {
		return err
	}
	if dispatching, exists := q.dispatching[sessionID]; exists && dispatching == id {
		return ErrEntryDispatching
	}
	q.entries[sessionID][index].Message = message.Clone()
	q.entries[sessionID][index].CommandID = commandID
	return nil
}

// RequeueDispatch assigns a fresh command identity to the exact reserved entry
// after the runtime definitively refuses its previous identity, then releases
// it back to FIFO ownership as one in-memory transition.
func (q *Queue) RequeueDispatch(sessionID string, previous, replacement agent.CommandID) error {
	if err := previous.Validate(); err != nil {
		return fmt.Errorf("prompt queue: previous command: %w", err)
	}
	if err := replacement.Validate(); err != nil {
		return fmt.Errorf("prompt queue: replacement command: %w", err)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	id, reserved := q.dispatching[sessionID]
	index := entryIndex(q.entries[sessionID], id)
	if !reserved || index < 0 {
		return ErrEntryNotFound
	}
	if q.entries[sessionID][index].CommandID != previous {
		return errors.New("prompt queue: dispatch command identity changed")
	}
	if slices.ContainsFunc(q.entries[sessionID], func(entry Entry) bool {
		return entry.ID != id && entry.CommandID == replacement
	}) {
		return fmt.Errorf("prompt queue: command %s is duplicated", replacement)
	}
	q.entries[sessionID][index].CommandID = replacement
	delete(q.dispatching, sessionID)
	return nil
}

func (q *Queue) Remove(sessionID string, id EntryID) (Entry, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := q.entries[sessionID]
	index := entryIndex(entries, id)
	if index < 0 {
		return Entry{}, ErrEntryNotFound
	}
	if dispatching, exists := q.dispatching[sessionID]; exists && dispatching == id {
		return Entry{}, ErrEntryDispatching
	}
	removed := cloneEntry(entries[index])
	q.removeAt(sessionID, index)
	return removed, nil
}

func (q *Queue) Move(sessionID string, id EntryID, offset int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := q.entries[sessionID]
	from := entryIndex(entries, id)
	if from < 0 {
		return ErrEntryNotFound
	}
	to := from + offset
	reserved, hasReservation := q.dispatching[sessionID]
	if offset == 0 || to < 0 || to >= len(entries) || (hasReservation && entries[from].ID == reserved) || (hasReservation && to == 0) {
		return ErrMoveUnavailable
	}
	entry := entries[from]
	entries = slices.Delete(entries, from, from+1)
	entries = slices.Insert(entries, to, entry)
	q.entries[sessionID] = entries
	return nil
}

// Promote moves an entry to the front without changing its stable identity.
// It is the queue-level half of "send now": orchestration decides whether a
// running turn must be canceled before the promoted entry can be dispatched.
func (q *Queue) Promote(sessionID string, id EntryID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := q.entries[sessionID]
	from := entryIndex(entries, id)
	if from < 0 {
		return ErrEntryNotFound
	}
	target := 0
	if dispatching, exists := q.dispatching[sessionID]; exists {
		if entries[from].ID == dispatching {
			return ErrEntryDispatching
		}
		target = 1
	}
	if from == target {
		return nil
	}
	entry := entries[from]
	entries = slices.Delete(entries, from, from+1)
	entries = slices.Insert(entries, target, entry)
	q.entries[sessionID] = entries
	return nil
}

func (q *Queue) Clear(sessionID string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := q.entries[sessionID]
	if len(entries) == 0 {
		return 0
	}
	count := len(entries)
	clear(entries)
	delete(q.entries, sessionID)
	delete(q.dispatching, sessionID)
	return count
}

func (q *Queue) ensureStorage() {
	if q.entries == nil {
		q.entries = make(map[string][]Entry)
	}
	if q.dispatching == nil {
		q.dispatching = make(map[string]EntryID)
	}
}

func (q *Queue) removeAt(sessionID string, index int) {
	entries := q.entries[sessionID]
	clear(entries[index : index+1])
	entries = slices.Delete(entries, index, index+1)
	if len(entries) == 0 {
		delete(q.entries, sessionID)
		return
	}
	q.entries[sessionID] = entries
}

func validateEntry(commandID agent.CommandID, sessionID string, message agent.Message, options agent.RunOptions) error {
	if err := commandID.Validate(); err != nil {
		return fmt.Errorf("prompt queue: %w", err)
	}
	if err := cliidentity.ValidateSession(sessionID); err != nil {
		return fmt.Errorf("%w: %v", ErrSessionIDRequired, err)
	}
	if err := message.Validate(); err != nil {
		return fmt.Errorf("prompt queue: %w", err)
	}
	if err := options.Validate(); err != nil {
		return fmt.Errorf("prompt queue: %w", err)
	}
	return nil
}

func entryIndex(entries []Entry, id EntryID) int {
	return slices.IndexFunc(entries, func(entry Entry) bool { return entry.ID == id })
}

func (q *Queue) allocateEntryIDs(count int) ([]EntryID, error) {
	if count < 0 || uint64(count) > math.MaxUint64-q.nextID {
		return nil, errors.New("prompt queue: entry identity space is exhausted")
	}
	ids := make([]EntryID, count)
	for index := range ids {
		q.nextID++
		ids[index] = EntryID{value: q.nextID}
	}
	return ids, nil
}

func cloneEntry(entry Entry) Entry {
	entry.SessionID = strings.Clone(entry.SessionID)
	entry.Message = entry.Message.Clone()
	entry.Options = entry.Options.Clone()
	return entry
}
