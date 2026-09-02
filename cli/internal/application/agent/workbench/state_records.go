package workbench

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

const (
	formatVersion     = 1
	maximumStateBytes = 16 << 20
)

type envelope[T any] struct {
	Version int `json:"version"`
	Value   T   `json:"value"`
}

type sessionState struct {
	SessionID       string                  `json:"sessionId"`
	Draft           agent.Message           `json:"draft"`
	PendingRuns     []PendingRun            `json:"pendingRuns"`
	PendingResume   *PendingResume          `json:"pendingResume,omitempty"`
	PendingRollback *PendingSessionRollback `json:"pendingRollback,omitempty"`
	PendingSteer    *pendingSteerRecord     `json:"pendingSteer,omitempty"`
}

func validateSessionDraft(draft agent.Message) error {
	if messageEmpty(draft) {
		return nil
	}
	if err := draft.Validate(); err != nil {
		return fmt.Errorf("session draft: %w", err)
	}
	return nil
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
	if err := validateStashes(s.stashes); err != nil {
		return fmt.Errorf("load prompt stashes: %w", err)
	}
	if err := s.loadOptional("workspaces.json", &s.workspaces); err != nil {
		return fmt.Errorf("load recent workspaces: %w", err)
	}
	if err := validateWorkspaces(s.workspaces); err != nil {
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

func (s *Store) load(name string, value any) error {
	if s.persistence == nil {
		return fs.ErrNotExist
	}
	body, err := s.persistence.Read(name, maximumStateBytes)
	if err != nil {
		return err
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
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Store) loadSessionStates() error {
	if s.persistence == nil {
		return nil
	}
	entries, err := s.persistence.List("sessions")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, name := range entries {
		if filepath.Ext(name) != ".json" {
			continue
		}
		if s.confirmedSessionStateFile(name) {
			continue
		}
		state, err := s.loadSessionState(name)
		if err != nil {
			return err
		}
		if err := s.restoreSessionState(name, state); err != nil {
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
	if err := runtimeprotocol.ValidateSessionID(state.SessionID); err != nil || name != filepath.Base(s.sessionStateName(state.SessionID)) {
		return sessionState{}, fmt.Errorf("state %s has an invalid session identity", name)
	}
	return state, nil
}

func (s *Store) restoreSessionState(name string, state sessionState) error {
	if err := validateSessionDraft(state.Draft); err != nil {
		return fmt.Errorf("state %s: %w", name, err)
	}
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
	if err := runtimeprotocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	if err := validateSessionDraft(draft); err != nil {
		return err
	}
	if pending, exists := s.sessionDeletions[sessionID]; exists && pending.Phase == SessionDeletionConfirmed {
		return errors.New("session authoring state has been retired")
	}
	name := s.sessionStateName(sessionID)
	if messageEmpty(draft) && len(pending) == 0 && resume == nil && rollback == nil && steer == nil {
		return s.remove(name)
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
	if s.writeBarrier != nil {
		return s.writeBarrier
	}
	if s.persistence == nil {
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
	return s.persistence.Replace(name, encoded)
}

func (s *Store) remove(name string) error {
	if s.writeBarrier != nil {
		return s.writeBarrier
	}
	if s.persistence == nil {
		return nil
	}
	return s.persistence.Remove(name)
}

func (s *Store) blockWrites(cause error) error {
	if s.writeBarrier == nil {
		s.writeBarrier = fmt.Errorf(
			"workbench persistence requires reopen after an incomplete transaction: %w",
			cause,
		)
	}
	return s.writeBarrier
}

func (s *Store) sessionStateName(sessionID string) string {
	digest := sha256.Sum256([]byte(sessionID))
	return filepath.Join("sessions", hex.EncodeToString(digest[:16])+".json")
}
