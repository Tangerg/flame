// Package workbench owns durable, CLI-local authoring state. It deliberately
// knows nothing about terminal widgets or runtime persistence.
package workbench

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

const (
	defaultHistoryCapacity   = 1000
	defaultStashCapacity     = 100
	defaultWorkspaceCapacity = 50
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

func (s Stash) Validate() error {
	identity, err := hex.DecodeString(s.ID)
	if err != nil || len(identity) != 8 || hex.EncodeToString(identity) != s.ID {
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
	if err := runtimeprotocol.ValidateSessionID(s.SessionID); err != nil {
		return fmt.Errorf("stash transfer: %w", err)
	}
	if err := s.Draft.Validate(); err != nil {
		return fmt.Errorf("stash transfer source: %w", err)
	}
	if err := s.Stash.Validate(); err != nil {
		return fmt.Errorf("stash transfer destination: %w", err)
	}
	if !s.Stash.Message.Equal(s.Draft) {
		return errors.New("stash transfer prompts are inconsistent")
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

func (w Workspace) Validate() error {
	canonical := filepath.Clean(strings.TrimSpace(w.Path))
	if canonical == "." || !filepath.IsAbs(canonical) || canonical != w.Path {
		return errors.New("workspace path must be canonical and absolute")
	}
	if w.LastOpened.IsZero() {
		return errors.New("workspace last-opened time is empty")
	}
	return nil
}

// historyEntry binds a runtime-accepted prompt to its mutation identity. Plain
// authoring history intentionally has no identity; accepted starts use it to
// make the history half of outbox settlement idempotent across process or
// filesystem failure between the two durable files.
type historyEntry struct {
	agent.Message
	CommandID agent.CommandID `json:"commandId,omitempty"`
}

// Store is the aggregate root for CLI authoring state. Every mutating method
// updates memory only after its durable replacement succeeds.
type Store struct {
	mu                sync.Mutex
	persistence       Persistence
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
	return newStore(nil, config)
}

// Open loads an explicitly durable Store through its filesystem-neutral port.
func Open(storage Persistence, config Config) (*Store, error) {
	if storage == nil {
		return nil, errors.New("workbench persistence is not configured")
	}
	store, err := newStore(storage, config)
	if err != nil {
		return nil, err
	}
	if err := store.loadState(); err != nil {
		return nil, err
	}
	return store, nil
}

func newStore(storage Persistence, config Config) (*Store, error) {
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
	if err := runtimeprotocol.ValidateSessionID(sessionID); err != nil {
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
	if err := stash.Validate(); err != nil {
		return Stash{}, err
	}
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
	if err := runtimeprotocol.ValidateSessionID(sessionID); err != nil {
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
	if err := stash.Validate(); err != nil {
		return Stash{}, err
	}
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
	workspace := Workspace{Path: path, LastOpened: s.now().UTC()}
	if err := workspace.Validate(); err != nil {
		return err
	}
	next := slices.DeleteFunc(slices.Clone(s.workspaces), func(item Workspace) bool { return item.Path == path })
	next = slices.Insert(next, 0, workspace)
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

func validateStashes(stashes []Stash) error {
	seen := make(map[string]struct{}, len(stashes))
	for index, stash := range stashes {
		if err := stash.Validate(); err != nil {
			return fmt.Errorf("stash %d: %w", index+1, err)
		}
		if _, duplicate := seen[stash.ID]; duplicate {
			return fmt.Errorf("stash %d repeats identity %s", index+1, stash.ID)
		}
		seen[stash.ID] = struct{}{}
	}
	return nil
}

func validateWorkspaces(workspaces []Workspace) error {
	seen := make(map[string]struct{}, len(workspaces))
	for index, workspace := range workspaces {
		if err := workspace.Validate(); err != nil {
			return fmt.Errorf("workspace %d: %w", index+1, err)
		}
		if _, duplicate := seen[workspace.Path]; duplicate {
			return fmt.Errorf("workspace %d repeats path %q", index+1, workspace.Path)
		}
		seen[workspace.Path] = struct{}{}
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
