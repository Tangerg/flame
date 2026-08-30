// Package workbench owns durable, CLI-local authoring state. It deliberately
// knows nothing about terminal widgets or runtime persistence.
package workbench

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/spf13/pathologize"

	"github.com/Tangerg/flame/cli/internal/agent"
	"github.com/Tangerg/flame/cli/internal/sessionidentity"
)

const (
	formatVersion            = 1
	defaultHistoryCapacity   = 1000
	defaultStashCapacity     = 100
	defaultWorkspaceCapacity = 50
	maximumStateBytes        = 16 << 20
	stashTransferName        = "stash-transfer.json"
	sessionDeletionsName     = "session-deletions.json"
)

// Config controls bounded state and supplies deterministic identity sources to
// tests. Absent capacities select production defaults; present capacities must
// be explicit positive values.
type Config struct {
	HistoryCapacity   *Capacity
	StashCapacity     *Capacity
	WorkspaceCapacity *Capacity
	Now               func() time.Time
	Random            io.Reader
}

// Stash is an explicitly named prompt snapshot.
type Stash struct {
	ID        string        `json:"id"`
	CreatedAt time.Time     `json:"createdAt"`
	Message   agent.Message `json:"message"`
}

// stashTransfer is the durable intent for the only workbench mutation that
// spans the global stash catalog and one session aggregate. Recovery uses the
// draft value as an ownership precondition: it completes an interrupted move
// only while that exact draft still occupies the source session.
type stashTransfer struct {
	SessionID string        `json:"sessionId"`
	Draft     agent.Message `json:"draft"`
	Stash     Stash         `json:"stash"`
}

func (s stashTransfer) validate() error {
	if _, err := sessionidentity.Parse(s.SessionID); err != nil {
		return fmt.Errorf("stash transfer: %w", err)
	}
	if messageEmpty(s.Draft) || messageEmpty(s.Stash.Message) {
		return errors.New("stash transfer prompt is empty")
	}
	identity, err := hex.DecodeString(s.Stash.ID)
	if err != nil || len(identity) != 8 || s.Stash.CreatedAt.IsZero() ||
		!s.Stash.Message.Equal(s.Draft) {
		return errors.New("stash transfer identity or prompt is inconsistent")
	}
	return nil
}

func stashEqual(left, right Stash) bool {
	return left.ID == right.ID && left.CreatedAt.Equal(right.CreatedAt) && left.Message.Equal(right.Message)
}

// Workspace is one recently used authoring root.
type Workspace struct {
	Path       string    `json:"path"`
	LastOpened time.Time `json:"lastOpened"`
}

// historyEntry binds a runtime-accepted prompt to its mutation identity. Plain
// authoring history intentionally has no identity; accepted starts use it to
// make the history half of outbox settlement idempotent across process or
// filesystem failure between the two durable files.
type historyEntry struct {
	agent.Message
	CommandID agent.CommandID `json:"commandId,omitempty"`
}

type sessionState struct {
	SessionID       string                  `json:"sessionId"`
	Draft           agent.Message           `json:"draft"`
	PendingRuns     []PendingRun            `json:"pendingRuns"`
	PendingResume   *PendingResume          `json:"pendingResume,omitempty"`
	PendingRollback *PendingSessionRollback `json:"pendingRollback,omitempty"`
	PendingSteer    *pendingSteerRecord     `json:"pendingSteer,omitempty"`
}

// Store is the aggregate root for CLI authoring state. Every mutating method
// updates memory only after its durable replacement succeeds.
type Store struct {
	mu                sync.Mutex
	persistence       persistence
	historyCapacity   int
	stashCapacity     int
	workspaceCapacity int
	now               func() time.Time
	random            io.Reader
	history           []historyEntry
	drafts            map[string]agent.Message
	stashes           []Stash
	workspaces        []Workspace
	pendingRuns       map[string][]PendingRun
	pendingResumes    map[string]PendingResume
	pendingRollbacks  map[string]PendingSessionRollback
	pendingSteers     map[string]PendingSteer
	sessionDeletions  map[string]PendingSessionDeletion
	draftTransfer     *DraftTransfer
}

// OpenMemory constructs an explicitly process-local Store.
func OpenMemory(config Config) (*Store, error) {
	return newStore(newMemoryPersistence(), config)
}

// OpenDirectory loads an explicitly durable directory-backed Store.
func OpenDirectory(directory string, config Config) (*Store, error) {
	storage, err := newDirectoryPersistence(directory)
	if err != nil {
		return nil, err
	}
	store, err := newStore(storage, config)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(storage.Directory(), 0o700); err != nil {
		return nil, fmt.Errorf("create workbench directory: %w", err)
	}
	if err := store.loadState(); err != nil {
		return nil, err
	}
	return store, nil
}

func newStore(storage persistence, config Config) (*Store, error) {
	if err := storage.Validate(); err != nil {
		return nil, err
	}
	historyCapacity, err := resolveCapacity(config.HistoryCapacity, defaultHistoryCapacity)
	if err != nil {
		return nil, fmt.Errorf("history capacity: %w", err)
	}
	stashCapacity, err := resolveCapacity(config.StashCapacity, defaultStashCapacity)
	if err != nil {
		return nil, fmt.Errorf("stash capacity: %w", err)
	}
	workspaceCapacity, err := resolveCapacity(config.WorkspaceCapacity, defaultWorkspaceCapacity)
	if err != nil {
		return nil, fmt.Errorf("workspace capacity: %w", err)
	}
	store := &Store{
		persistence:       storage,
		historyCapacity:   historyCapacity,
		stashCapacity:     stashCapacity,
		workspaceCapacity: workspaceCapacity,
		now:               config.Now,
		random:            config.Random,
		drafts:            make(map[string]agent.Message),
		pendingRuns:       make(map[string][]PendingRun),
		pendingResumes:    make(map[string]PendingResume),
		pendingRollbacks:  make(map[string]PendingSessionRollback),
		pendingSteers:     make(map[string]PendingSteer),
		sessionDeletions:  make(map[string]PendingSessionDeletion),
	}
	if store.now == nil {
		store.now = time.Now
	}
	if store.random == nil {
		store.random = rand.Reader
	}
	return store, nil
}

func (s *Store) loadState() error {
	if err := s.loadOptional("history.json", &s.history); err != nil {
		return fmt.Errorf("load prompt history: %w", err)
	}
	if err := validateHistory(s.history); err != nil {
		return fmt.Errorf("load prompt history: %w", err)
	}
	if err := s.loadOptional("stashes.json", &s.stashes); err != nil {
		return fmt.Errorf("load prompt stashes: %w", err)
	}
	if err := s.loadOptional("workspaces.json", &s.workspaces); err != nil {
		return fmt.Errorf("load recent workspaces: %w", err)
	}
	if err := s.loadSessionDeletions(); err != nil {
		return err
	}
	if err := s.loadSessionStates(); err != nil {
		return fmt.Errorf("load session authoring state: %w", err)
	}
	if err := s.loadDraftTransfer(); err != nil {
		return err
	}
	s.recoverConfirmedSessionDeletions()
	s.history = s.trimHistory(s.history)
	s.stashes = tailStashes(s.stashes, s.stashCapacity)
	s.workspaces = slices.Clone(s.workspaces[:min(len(s.workspaces), s.workspaceCapacity)])
	if err := s.recoverStashTransfer(); err != nil {
		return fmt.Errorf("recover prompt stash transfer: %w", err)
	}
	return nil
}

func (s *Store) loadDraftTransfer() error {
	var draftTransfer *DraftTransfer
	if err := s.loadOptional(sessionDraftTransferName, &draftTransfer); err != nil {
		return fmt.Errorf("load session draft transfer: %w", err)
	}
	if draftTransfer == nil {
		return nil
	}
	cloned := draftTransfer.clone()
	s.draftTransfer = &cloned
	if err := s.recoverDraftTransfer(); err != nil {
		return fmt.Errorf("recover session draft transfer: %w", err)
	}
	return nil
}

// History returns detached prompts in oldest-to-newest order.
func (s *Store) History() []agent.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := make([]agent.Message, len(s.history))
	for index, entry := range s.history {
		messages[index] = entry.Clone()
	}
	return messages
}

// Remember records a submitted or deliberately cleared prompt.
func (s *Store) Remember(message agent.Message) error {
	message = message.Clone()
	if messageEmpty(message) {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.history) > 0 && s.history[len(s.history)-1].Equal(message) {
		return nil
	}
	next := append(cloneHistory(s.history), historyEntry{Message: message})
	next = s.trimHistory(next)
	if err := s.save("history.json", next); err != nil {
		return err
	}
	s.history = next
	return nil
}

// Draft loads a session-specific prompt without consuming it.
func (s *Store) Draft(sessionID string) (agent.Message, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	draft := s.drafts[sessionID]
	return draft.Clone(), !messageEmpty(draft), nil
}

// SaveDraft atomically replaces a session draft, or removes it when empty.
func (s *Store) SaveDraft(sessionID string, message agent.Message) error {
	if _, err := sessionidentity.Parse(sessionID); err != nil {
		return err
	}
	message = message.Clone()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, present := s.drafts[sessionID]
	if (present && current.Equal(message)) || (!present && messageEmpty(message)) {
		return nil
	}
	if err := s.saveSessionState(sessionID, message, s.pendingRuns[sessionID]); err != nil {
		return err
	}
	if messageEmpty(message) {
		delete(s.drafts, sessionID)
	} else {
		s.drafts[sessionID] = message
	}
	return nil
}

// DiscardDraft retires authoring state for a session that no longer exists.
// It is intentionally distinct from saving an empty draft at call sites: the
// caller is expressing a lifecycle transition, not an editor value change.
func (s *Store) DiscardDraft(sessionID string) error {
	return s.SaveDraft(sessionID, agent.Message{})
}

// StashPrompt preserves a prompt independently of its session draft.
func (s *Store) StashPrompt(message agent.Message) (Stash, error) {
	message = message.Clone()
	if messageEmpty(message) {
		return Stash{}, errors.New("cannot stash an empty prompt")
	}
	identity := make([]byte, 8)
	if _, err := io.ReadFull(s.random, identity); err != nil {
		return Stash{}, fmt.Errorf("create stash id: %w", err)
	}
	stash := Stash{ID: hex.EncodeToString(identity), CreatedAt: s.now().UTC(), Message: message}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := append(slices.Clone(s.stashes), stash)
	next = tailStashes(next, s.stashCapacity)
	if err := s.save("stashes.json", next); err != nil {
		return Stash{}, err
	}
	s.stashes = next
	return cloneStash(stash), nil
}

// StashDraft transfers one session draft into the bounded stash collection.
// A durable intent makes the cross-file move restart-safe; synchronous failure
// restores the complete pre-transaction stash collection so capacity eviction
// cannot turn compensation into data loss.
func (s *Store) StashDraft(sessionID string, message agent.Message) (Stash, error) {
	if _, err := sessionidentity.Parse(sessionID); err != nil {
		return Stash{}, err
	}
	message = message.Clone()
	if messageEmpty(message) {
		return Stash{}, errors.New("cannot stash an empty prompt")
	}
	identity := make([]byte, 8)
	if _, err := io.ReadFull(s.random, identity); err != nil {
		return Stash{}, fmt.Errorf("create stash id: %w", err)
	}
	stash := Stash{ID: hex.EncodeToString(identity), CreatedAt: s.now().UTC(), Message: message}
	transfer := stashTransfer{SessionID: sessionID, Draft: message, Stash: stash}

	s.mu.Lock()
	defer s.mu.Unlock()
	current, present := s.drafts[sessionID]
	if !present || !current.Equal(message) {
		return Stash{}, errors.New("session draft changed before it could be stashed")
	}
	previous := slices.Clone(s.stashes)
	next := tailStashes(append(slices.Clone(s.stashes), stash), s.stashCapacity)
	if err := s.save(stashTransferName, transfer); err != nil {
		return Stash{}, fmt.Errorf("save stash transfer: %w", err)
	}
	if err := s.save("stashes.json", next); err != nil {
		_ = s.remove(stashTransferName)
		return Stash{}, err
	}
	if err := s.saveSessionState(sessionID, agent.Message{}, s.pendingRuns[sessionID]); err != nil {
		rollbackErr := s.save("stashes.json", previous)
		if rollbackErr != nil {
			s.stashes = next
			return Stash{}, errors.Join(
				fmt.Errorf("clear session draft: %w", err),
				fmt.Errorf("restore prompt stashes: %w", rollbackErr),
			)
		}
		if cleanupErr := s.remove(stashTransferName); cleanupErr != nil {
			// Re-publish the intended stash so a surviving journal always describes
			// a forward-recoverable state rather than reviving a rolled-back move.
			if restoreErr := s.save("stashes.json", next); restoreErr != nil {
				return Stash{}, errors.Join(
					fmt.Errorf("clear session draft: %w", err),
					fmt.Errorf("remove stash transfer: %w", cleanupErr),
					fmt.Errorf("restore recoverable stash: %w", restoreErr),
				)
			}
			s.stashes = next
			return Stash{}, errors.Join(
				fmt.Errorf("clear session draft: %w", err),
				fmt.Errorf("remove stash transfer: %w", cleanupErr),
			)
		}
		return Stash{}, fmt.Errorf("clear session draft: %w", err)
	}
	s.stashes = next
	delete(s.drafts, sessionID)
	// Once the source draft is absent, the transfer is committed. A stale
	// journal is harmless: recovery sees no matching source owner and only
	// retries this cleanup.
	_ = s.remove(stashTransferName)
	return cloneStash(stash), nil
}

func (s *Store) recoverStashTransfer() error {
	var transfer stashTransfer
	if err := s.loadOptional(stashTransferName, &transfer); err != nil {
		return err
	}
	if transfer.SessionID == "" {
		return nil
	}
	if err := transfer.validate(); err != nil {
		return err
	}
	current, present := s.drafts[transfer.SessionID]
	if present && current.Equal(transfer.Draft) {
		index := slices.IndexFunc(s.stashes, func(stash Stash) bool { return stash.ID == transfer.Stash.ID })
		switch {
		case index >= 0 && !stashEqual(s.stashes[index], transfer.Stash):
			return errors.New("stash transfer identity belongs to another prompt")
		case index < 0:
			next := tailStashes(append(slices.Clone(s.stashes), transfer.Stash), s.stashCapacity)
			if err := s.save("stashes.json", next); err != nil {
				return fmt.Errorf("save recovered prompt stash: %w", err)
			}
			s.stashes = next
		}
		if err := s.saveSessionState(transfer.SessionID, agent.Message{}, s.pendingRuns[transfer.SessionID]); err != nil {
			return fmt.Errorf("retire recovered session draft: %w", err)
		}
		delete(s.drafts, transfer.SessionID)
	}
	// The move is already complete when the source is absent, and a newer draft
	// means the old intent no longer owns that session value. Either state makes
	// replay unnecessary; cleanup is best-effort because the journal is
	// idempotent under both conditions.
	_ = s.remove(stashTransferName)
	return nil
}

// Stashes returns newest prompts first.
func (s *Store) Stashes() []Stash {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Stash, len(s.stashes))
	for i, stash := range slices.Backward(s.stashes) {
		out[len(s.stashes)-1-i] = cloneStash(stash)
	}
	return out
}

// Stash returns one detached prompt by identity.
func (s *Store) Stash(id string) (Stash, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, stash := range s.stashes {
		if stash.ID == id {
			return cloneStash(stash), true
		}
	}
	return Stash{}, false
}

// DeleteStash permanently removes one stash.
func (s *Store) DeleteStash(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := slices.DeleteFunc(slices.Clone(s.stashes), func(stash Stash) bool { return stash.ID == id })
	if len(next) == len(s.stashes) {
		return false, nil
	}
	if err := s.save("stashes.json", next); err != nil {
		return false, err
	}
	s.stashes = next
	return true, nil
}

// RememberWorkspace moves a workspace to the front of the recent list.
func (s *Store) RememberWorkspace(path string) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || !filepath.IsAbs(path) {
		return errors.New("workspace path must be absolute")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := slices.DeleteFunc(slices.Clone(s.workspaces), func(item Workspace) bool { return item.Path == path })
	next = slices.Insert(next, 0, Workspace{Path: path, LastOpened: s.now().UTC()})
	next = next[:min(len(next), s.workspaceCapacity)]
	if err := s.save("workspaces.json", next); err != nil {
		return err
	}
	s.workspaces = next
	return nil
}

// Workspaces returns recent workspaces in newest-first order.
func (s *Store) Workspaces() []Workspace {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.workspaces)
}

type envelope[T any] struct {
	Version int `json:"version"`
	Value   T   `json:"value"`
}

func (s *Store) load(name string, value any) error {
	if !s.persistence.Durable() {
		return os.ErrNotExist
	}
	file, err := os.Open(s.path(name))
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	body, err := io.ReadAll(io.LimitReader(file, maximumStateBytes+1))
	if err != nil {
		return fmt.Errorf("read workbench state %q: %w", name, err)
	}
	if len(body) > maximumStateBytes {
		return fmt.Errorf("workbench state %q exceeds %d bytes", name, maximumStateBytes)
	}
	var raw envelope[json.RawMessage]
	if err := decodeStateJSON(body, &raw); err != nil {
		return fmt.Errorf("decode workbench state %q: %w", name, err)
	}
	if raw.Version != formatVersion {
		return fmt.Errorf("unsupported workbench format %d", raw.Version)
	}
	if err := decodeStateJSON(raw.Value, value); err != nil {
		return fmt.Errorf("decode workbench state %q value: %w", name, err)
	}
	return nil
}

func decodeStateJSON(encoded []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func (s *Store) loadOptional(name string, value any) error {
	err := s.load(name, value)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Store) loadSessionStates() error {
	if !s.persistence.Durable() {
		return nil
	}
	directory := s.path("sessions")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if s.confirmedSessionStateFile(entry.Name()) {
			continue
		}
		state, err := s.loadSessionState(entry.Name())
		if err != nil {
			return err
		}
		if err := s.restoreSessionState(entry.Name(), state); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) loadSessionState(name string) (sessionState, error) {
	var state sessionState
	if err := s.load(filepath.Join("sessions", name), &state); err != nil {
		return sessionState{}, fmt.Errorf("load %s: %w", name, err)
	}
	if _, err := sessionidentity.Parse(state.SessionID); err != nil || name != filepath.Base(s.sessionStateName(state.SessionID)) {
		return sessionState{}, fmt.Errorf("state %s has an invalid session identity", name)
	}
	return state, nil
}

func (s *Store) restoreSessionState(name string, state sessionState) error {
	if err := validatePendingRunSequence(state.SessionID, state.PendingRuns); err != nil {
		return fmt.Errorf("state %s: %w", name, err)
	}
	if !messageEmpty(state.Draft) {
		s.drafts[state.SessionID] = state.Draft.Clone()
	}
	if len(state.PendingRuns) > 0 {
		s.pendingRuns[state.SessionID] = clonePendingRunSlice(state.PendingRuns)
	}
	if state.PendingResume != nil {
		if err := state.PendingResume.validate(); err != nil {
			return fmt.Errorf("state %s pending resume: %w", name, err)
		}
		s.pendingResumes[state.SessionID] = clonePendingResume(*state.PendingResume)
	}
	if state.PendingRollback != nil {
		pending := state.PendingRollback.clone()
		if err := pending.Validate(); err != nil {
			return fmt.Errorf("state %s pending rollback: %w", name, err)
		}
		if pending.SessionID != state.SessionID {
			return fmt.Errorf("state %s pending rollback belongs to another session", name)
		}
		s.pendingRollbacks[state.SessionID] = pending
	}
	if state.PendingSteer != nil {
		pending, err := restorePendingSteer(*state.PendingSteer)
		if err != nil {
			return fmt.Errorf("state %s pending steer: %w", name, err)
		}
		if err := pending.validateSession(state.SessionID); err != nil {
			return fmt.Errorf("state %s pending steer: %w", name, err)
		}
		s.pendingSteers[state.SessionID] = pending
	}
	return nil
}

func (s *Store) saveSessionState(sessionID string, draft agent.Message, pending []PendingRun) error {
	resume, ok := s.pendingResumes[sessionID]
	if !ok {
		return s.saveSessionStateWithResume(sessionID, draft, pending, nil)
	}
	return s.saveSessionStateWithResume(sessionID, draft, pending, &resume)
}

func (s *Store) saveSessionStateWithResume(
	sessionID string,
	draft agent.Message,
	pending []PendingRun,
	resume *PendingResume,
) error {
	var rollback *PendingSessionRollback
	if pendingRollback, exists := s.pendingRollbacks[sessionID]; exists {
		cloned := pendingRollback.clone()
		rollback = &cloned
	}
	var steer *PendingSteer
	if pendingSteer, exists := s.pendingSteers[sessionID]; exists {
		cloned := pendingSteer.clone()
		steer = &cloned
	}
	return s.saveSessionStateRecord(sessionID, draft, pending, resume, rollback, steer)
}

func (s *Store) saveSessionStateRecord(
	sessionID string,
	draft agent.Message,
	pending []PendingRun,
	resume *PendingResume,
	rollback *PendingSessionRollback,
	steer *PendingSteer,
) error {
	if s.draftTransferBlocks(sessionID) {
		return errors.New("session draft transfer requires recovery")
	}
	return s.saveSessionStateRecordUnfenced(sessionID, draft, pending, resume, rollback, steer)
}

func (s *Store) saveSessionStateRecordUnfenced(
	sessionID string,
	draft agent.Message,
	pending []PendingRun,
	resume *PendingResume,
	rollback *PendingSessionRollback,
	steer *PendingSteer,
) error {
	if _, err := sessionidentity.Parse(sessionID); err != nil {
		return err
	}
	if pending, exists := s.sessionDeletions[sessionID]; exists && pending.Phase == SessionDeletionConfirmed {
		return errors.New("session authoring state has been retired")
	}
	name := s.sessionStateName(sessionID)
	if messageEmpty(draft) && len(pending) == 0 && resume == nil && rollback == nil && steer == nil {
		if !s.persistence.Durable() {
			return nil
		}
		err := os.Remove(s.path(name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	state := sessionState{
		SessionID: sessionID, Draft: draft.Clone(), PendingRuns: clonePendingRunSlice(pending),
	}
	if resume != nil {
		cloned := clonePendingResume(*resume)
		state.PendingResume = &cloned
	}
	if rollback != nil {
		cloned := rollback.clone()
		state.PendingRollback = &cloned
	}
	if steer != nil {
		record := steer.record()
		state.PendingSteer = &record
	}
	return s.save(name, state)
}

func (s *Store) save(name string, value any) error {
	if !s.persistence.Durable() {
		return nil
	}
	encoded, err := json.MarshalIndent(envelope[any]{Version: formatVersion, Value: value}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state snapshot: %w", err)
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maximumStateBytes {
		return fmt.Errorf("workbench state %q exceeds %d bytes", name, maximumStateBytes)
	}
	path := s.path(name)
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".flame-state-*")
	if err != nil {
		return fmt.Errorf("create state snapshot: %w", err)
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write state snapshot: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync state snapshot: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close state snapshot: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace state snapshot: %w", err)
	}
	removeTemporary = false
	return nil
}

func (s *Store) remove(name string) error {
	if !s.persistence.Durable() {
		return nil
	}
	err := os.Remove(s.path(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Store) path(name string) string { return pathologize.Join(s.persistence.Directory(), name) }

func (s *Store) sessionStateName(sessionID string) string {
	digest := sha256.Sum256([]byte(sessionID))
	return filepath.Join("sessions", hex.EncodeToString(digest[:16])+".json")
}

func validateHistory(history []historyEntry) error {
	seen := make(map[agent.CommandID]struct{}, len(history))
	for index, entry := range history {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("entry %d: %w", index+1, err)
		}
		if entry.CommandID == "" {
			continue
		}
		if err := entry.CommandID.Validate(); err != nil {
			return fmt.Errorf("entry %d command: %w", index+1, err)
		}
		if _, duplicate := seen[entry.CommandID]; duplicate {
			return fmt.Errorf("entry %d repeats command %s", index+1, entry.CommandID)
		}
		seen[entry.CommandID] = struct{}{}
	}
	return nil
}

func (s *Store) trimHistory(history []historyEntry) []historyEntry {
	if len(history) <= s.historyCapacity {
		return cloneHistory(history)
	}
	pinned := make(map[agent.CommandID]struct{})
	for _, commands := range s.pendingRuns {
		for _, pending := range commands {
			pinned[pending.Command.CommandID] = struct{}{}
		}
	}
	pinnedHistory := 0
	for _, entry := range history {
		if _, protected := pinned[entry.CommandID]; protected {
			pinnedHistory++
		}
	}
	nonPinnedBudget := s.historyCapacity
	keepNonPinned := make(map[int]struct{}, nonPinnedBudget)
	for index := len(history) - 1; index >= 0 && len(keepNonPinned) < nonPinnedBudget; index-- {
		if _, protected := pinned[history[index].CommandID]; !protected {
			keepNonPinned[index] = struct{}{}
		}
	}
	trimmed := make([]historyEntry, 0, min(len(history), s.historyCapacity+pinnedHistory))
	for index, entry := range history {
		_, protected := pinned[entry.CommandID]
		_, recent := keepNonPinned[index]
		if protected || recent {
			trimmed = append(trimmed, historyEntry{Message: entry.Clone(), CommandID: entry.CommandID})
		}
	}
	return trimmed
}

func tailStashes(stashes []Stash, limit int) []Stash {
	if len(stashes) > limit {
		stashes = stashes[len(stashes)-limit:]
	}
	out := make([]Stash, len(stashes))
	for i, stash := range stashes {
		out[i] = cloneStash(stash)
	}
	return out
}

func cloneHistory(history []historyEntry) []historyEntry {
	out := make([]historyEntry, len(history))
	for index, entry := range history {
		out[index] = historyEntry{Message: entry.Clone(), CommandID: entry.CommandID}
	}
	return out
}

func clonePendingRuns(pending map[string][]PendingRun) map[string][]PendingRun {
	out := make(map[string][]PendingRun, len(pending))
	for sessionID, commands := range pending {
		out[sessionID] = clonePendingRunSlice(commands)
	}
	return out
}

func clonePendingResumes(pending map[string]PendingResume) map[string]PendingResume {
	out := make(map[string]PendingResume, len(pending))
	for sessionID, command := range pending {
		out[sessionID] = clonePendingResume(command)
	}
	return out
}

func clonePendingRunSlice(commands []PendingRun) []PendingRun {
	out := make([]PendingRun, len(commands))
	for index, command := range commands {
		out[index] = clonePendingRun(command)
	}
	return out
}

func clonePendingRun(command PendingRun) PendingRun {
	command.Command = command.Command.Clone()
	return command
}

func pendingRunEqual(left, right PendingRun) bool {
	return left.State == right.State && left.Replay == right.Replay &&
		left.CancelCommandID == right.CancelCommandID && left.CancelReplay == right.CancelReplay &&
		left.Command.Equal(right.Command)
}

func pendingResumeEqual(left, right PendingResume) bool {
	return left.Command.Equal(right.Command) && left.Replay == right.Replay &&
		agent.InteractionsEqual(left.Interactions, right.Interactions)
}

func clonePendingResume(pending PendingResume) PendingResume {
	pending.Command = pending.Command.Clone()
	pending.Interactions = agent.CloneInteractions(pending.Interactions)
	return pending
}

func pendingRunIndex(commands []PendingRun, commandID agent.CommandID) int {
	return slices.IndexFunc(commands, func(command PendingRun) bool { return command.Command.CommandID == commandID })
}

func cloneStash(stash Stash) Stash {
	stash.Message = stash.Message.Clone()
	return stash
}

func messageEmpty(message agent.Message) bool {
	return strings.TrimSpace(message.Text) == "" && len(message.Attachments) == 0
}
