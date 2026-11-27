package workbench

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

var ErrUnavailable = errors.New("CLI workbench is unavailable")

const (
	maxHistoryEntries    = 1000
	maxStashes           = 100
	maxRecentWorkspaces  = 50
	stashIDEntropyBytes  = 8
	historyName          = "history"
	workspacesName       = "workspaces"
	stashesName          = "stashes"
	sessionDeletionsName = "session-deletions"
)

type Stash struct {
	ID        string            `json:"id"`
	CreatedAt time.Time         `json:"createdAt"`
	Message   authoring.Message `json:"message"`
}

func (s Stash) Validate() error {
	identity, err := hex.DecodeString(s.ID)
	if err != nil || len(identity) != stashIDEntropyBytes || hex.EncodeToString(identity) != s.ID {
		return errors.New("stash identity is not canonical")
	}
	if s.CreatedAt.IsZero() {
		return errors.New("stash creation time is empty")
	}
	if err := s.Message.Validate(); err != nil {
		return fmt.Errorf("stash prompt: %w", err)
	}
	return nil
}

type Workspace struct {
	Path       string    `json:"path"`
	LastOpened time.Time `json:"lastOpened"`
}

func (w Workspace) Validate() error {
	if err := protocol.ValidateWireTree((protocol.WorkspaceRef{Path: w.Path})); err != nil {
		return fmt.Errorf("workspace reference: %w", err)
	}
	if w.LastOpened.IsZero() {
		return errors.New("workspace last-opened time is empty")
	}
	return nil
}

type Store struct {
	mu                sync.Mutex
	persistence       Persistence
	history           []authoring.Message
	drafts            map[string]authoring.Message
	stashes           []Stash
	workspaces        []Workspace
	queue             Queue
	pendingResumes    map[string]PendingResume
	pendingRollbacks  map[string]PendingSessionRollback
	pendingSteers     map[string]PendingSteer
	sessionDeletions  map[string]PendingSessionDeletion
	installedCommands map[string]PendingInstalledCommand
	closed            bool
	closeErr          error
}

func OpenMemory() *Store {
	return newStore(newMemoryStorage())
}

func Open(storage Persistence) (*Store, error) {
	if storage == nil {
		return nil, errors.New("workbench persistence is not configured")
	}
	store := newStore(storage)
	if err := store.loadState(); err != nil {
		return nil, errors.Join(err, storage.Close())
	}
	return store, nil
}

func newStore(storage Persistence) *Store {
	store := &Store{
		persistence:       storage,
		drafts:            make(map[string]authoring.Message),
		pendingResumes:    make(map[string]PendingResume),
		pendingRollbacks:  make(map[string]PendingSessionRollback),
		pendingSteers:     make(map[string]PendingSteer),
		sessionDeletions:  make(map[string]PendingSessionDeletion),
		installedCommands: make(map[string]PendingInstalledCommand),
	}
	store.queue = Queue{store: store, bySession: make(map[string][]PendingRun)}
	return store
}

func (s *Store) Queue() *Queue { return &s.queue }

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	s.closeErr = s.persistence.Close()
	return s.closeErr
}

func (s *Store) History() []authoring.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := make([]authoring.Message, len(s.history))
	for index, entry := range s.history {
		messages[index] = entry.Clone()
	}
	return messages
}

func (s *Store) MoveDraftToHistory(sessionID string, message authoring.Message) error {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	if err := message.Validate(); err != nil {
		return fmt.Errorf("move draft to history: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	update := s.sessionUpdate(sessionID)
	update.draft = authoring.Message{}
	return s.recordPromptSettlement(update, message)
}

func (s *Store) Draft(sessionID string) (authoring.Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	draft := s.drafts[sessionID]
	return draft.Clone(), !draft.IsEmpty()
}

func (s *Store) SaveDraft(sessionID string, message authoring.Message) error {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	message = message.Clone()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, present := s.drafts[sessionID]
	if (present && current.Equal(message)) || (!present && message.IsEmpty()) {
		return nil
	}
	return s.saveSessionState(sessionID, message, s.queue.bySession[sessionID])
}

func (s *Store) newStash(message authoring.Message) (Stash, error) {
	message = message.Clone()
	if err := message.Validate(); err != nil {
		return Stash{}, fmt.Errorf("stash prompt: %w", err)
	}
	var identity [stashIDEntropyBytes]byte
	rand.Read(identity[:])
	id := hex.EncodeToString(identity[:])
	if slices.ContainsFunc(s.stashes, func(stash Stash) bool { return stash.ID == id }) {
		return Stash{}, fmt.Errorf("stash identity %q is already in use", id)
	}
	return Stash{ID: id, CreatedAt: time.Now().UTC(), Message: message}, nil
}

func (s *Store) StashDraft(sessionID string, message authoring.Message) (Stash, error) {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return Stash{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stash, err := s.newStash(message)
	if err != nil {
		return Stash{}, err
	}
	next := retainedStashes(append(slices.Clone(s.stashes), stash))
	record, err := encodeRecord(stashesName, next)
	if err != nil {
		return Stash{}, err
	}
	update := s.sessionUpdate(sessionID)
	update.draft = authoring.Message{}
	if err := s.commitSessions([]sessionUpdate{update}, record); err != nil {
		return Stash{}, err
	}
	s.stashes = next
	return cloneStash(stash), nil
}

func (s *Store) Stashes() []Stash {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Stash, len(s.stashes))
	for i, stash := range slices.Backward(s.stashes) {
		out[len(s.stashes)-1-i] = cloneStash(stash)
	}
	return out
}

func (s *Store) DeleteStash(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := slices.DeleteFunc(slices.Clone(s.stashes), func(stash Stash) bool { return stash.ID == id })
	if len(next) == len(s.stashes) {
		return false, nil
	}
	if err := s.save(stashesName, next, func() { s.stashes = next }); err != nil {
		return false, err
	}

	return true, nil
}

func (s *Store) Workspaces() []Workspace {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.workspaces)
}

func validateStashes(stashes []Stash) error {
	return validateDistinct("stash", stashes, func(stash Stash) string { return stash.ID })
}

func validateWorkspaces(workspaces []Workspace) error {
	return validateDistinct("workspace", workspaces, func(workspace Workspace) string { return workspace.Path })
}

func validateDistinct[T interface{ Validate() error }](noun string, records []T, identity func(T) string) error {
	seen := make(map[string]struct{}, len(records))
	for index, record := range records {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("%s %d: %w", noun, index+1, err)
		}
		key := identity(record)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%s %d repeats %q", noun, index+1, key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (s *Store) historyWith(message authoring.Message) ([]authoring.Message, bool) {
	if len(s.history) > 0 && s.history[len(s.history)-1].Equal(message) {
		return nil, false
	}
	start := max(0, len(s.history)-maxHistoryEntries+1)
	return append(slices.Clone(s.history[start:]), message.Clone()), true
}

func (s *Store) recordPromptSettlement(update sessionUpdate, message authoring.Message) error {
	history, changed := s.historyWith(message)
	if !changed {
		return s.commitSessions([]sessionUpdate{update})
	}
	record, err := encodeRecord(historyName, history)
	if err != nil {
		return err
	}
	if err := s.commitSessions([]sessionUpdate{update}, record); err != nil {
		return err
	}
	s.history = history
	return nil
}

func retainedStashes(stashes []Stash) []Stash {
	stashes = stashes[max(0, len(stashes)-maxStashes):]
	out := make([]Stash, len(stashes))
	for i, stash := range stashes {
		out[i] = cloneStash(stash)
	}
	return out
}

func clonePendingRunSlice(commands []PendingRun) []PendingRun {
	out := make([]PendingRun, len(commands))
	for index, command := range commands {
		out[index] = command.clone()
	}
	return out
}

func pendingResumeEqual(left, right PendingResume) bool {
	return left.Command.Equal(right.Command) && left.Replay == right.Replay &&
		conversation.InterruptsEqual(left.Interrupts, right.Interrupts)
}

func clonePendingResume(pending PendingResume) PendingResume {
	pending.Command = pending.Command.Clone()
	pending.Interrupts = conversation.CloneInterrupts(pending.Interrupts)
	return pending
}

func cloneStash(stash Stash) Stash {
	stash.Message = stash.Message.Clone()
	return stash
}
