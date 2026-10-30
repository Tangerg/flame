package workbench

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem/commit"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	maximumStateBytes     = 16 << 20
	sessionStateExtension = ".json"
)

type sessionState struct {
	SessionID       string                  `json:"sessionId"`
	Draft           authoring.Message       `json:"draft"`
	PendingRuns     []PendingRun            `json:"pendingRuns"`
	PendingResume   *PendingResume          `json:"pendingResume,omitzero"`
	PendingRollback *PendingSessionRollback `json:"pendingRollback,omitzero"`
	PendingSteer    *pendingSteerRecord     `json:"pendingSteer,omitzero"`
}

func validateSessionDraft(draft authoring.Message) error {
	if draft.IsEmpty() {
		return nil
	}
	if err := draft.Validate(); err != nil {
		return fmt.Errorf("session draft: %w", err)
	}
	return nil
}

func (s *Store) loadState() error {
	if err := s.loadOptional(historyName, &s.history); err != nil {
		return fmt.Errorf("load prompt history: %w", err)
	}
	if err := validateHistory(s.history); err != nil {
		return fmt.Errorf("load prompt history: %w", err)
	}
	if err := s.loadOptional(stashesName, &s.stashes); err != nil {
		return fmt.Errorf("load prompt stashes: %w", err)
	}
	if err := validateStashes(s.stashes); err != nil {
		return fmt.Errorf("load prompt stashes: %w", err)
	}
	if err := s.loadOptional(workspacesName, &s.workspaces); err != nil {
		return fmt.Errorf("load recent workspaces: %w", err)
	}
	if err := validateWorkspaces(s.workspaces); err != nil {
		return fmt.Errorf("load recent workspaces: %w", err)
	}
	if err := s.loadSessionDeletions(); err != nil {
		return err
	}
	if err := s.loadInstalledCommands(); err != nil {
		return err
	}
	if err := s.loadSessionStates(); err != nil {
		return fmt.Errorf("load session authoring state: %w", err)
	}
	if err := s.loadDraftTransfer(); err != nil {
		return err
	}
	if err := s.recoverConfirmedSessionDeletions(); err != nil {
		return fmt.Errorf("recover retired sessions: %w", err)
	}
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
	if err := decodeStateJSON(body, value); err != nil {
		return fmt.Errorf("decode workbench state %q: %w", name, err)
	}
	return nil
}

func decodeStateJSON(encoded []byte, value any) error {
	return json.Unmarshal(encoded, value, stateJSONOptions)
}

// A durable state record names its own representations. encoding/json/v2 has no
// decided JSON form for time.Duration and refuses to guess, so the store states
// the one its files already contain: whole nanoseconds. Deterministic member
// order keeps a rewritten file diffable, and an unknown member is a record this
// build cannot honor rather than one to ignore.
var stateJSONOptions = json.JoinOptions(
	json.Deterministic(true),
	json.RejectUnknownMembers(true),
	// An absent collection stays null so a reloaded record still distinguishes
	// "not answered yet" from "answered with nothing".
	json.FormatNilSliceAsNull(true),
	json.FormatNilMapAsNull(true),
	json.WithMarshalers(json.MarshalToFunc(func(encoder *jsontext.Encoder, value time.Duration) error {
		return encoder.WriteToken(jsontext.Int(int64(value)))
	})),
	json.WithUnmarshalers(json.UnmarshalFromFunc(func(decoder *jsontext.Decoder, value *time.Duration) error {
		token, err := decoder.ReadToken()
		if err != nil {
			return err
		}
		nanoseconds, err := token.Int()
		if err != nil {
			return err
		}
		*value = time.Duration(nanoseconds)
		return nil
	})),
)

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
	entries, err := s.persistence.ListFiles("sessions", sessionStateExtension)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, name := range entries {
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
	if err := protocol.ValidateSessionID(state.SessionID); err != nil || name != filepath.Base(s.sessionStateName(state.SessionID)) {
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
	for index := range state.PendingRuns {
		pending := &state.PendingRuns[index]
		pending.Command.Input, pending.inputFailure = s.readInput(pending.InputDigest)
	}
	if state.PendingResume != nil {
		if err := state.PendingResume.validate(); err != nil {
			return fmt.Errorf("state %s pending resume: %w", name, err)
		}
		state.PendingResume.Command.Input, state.PendingResume.inputFailure = s.readInput(state.PendingResume.InputDigest)
	}
	if state.PendingRollback != nil {
		if err := state.PendingRollback.Validate(); err != nil {
			return fmt.Errorf("state %s pending rollback: %w", name, err)
		}
		if state.PendingRollback.SessionID != state.SessionID {
			return fmt.Errorf("state %s pending rollback belongs to another session", name)
		}
	}
	var steer *PendingSteer
	if state.PendingSteer != nil {
		pending, err := restorePendingSteer(*state.PendingSteer)
		if err != nil {
			return fmt.Errorf("state %s pending steer: %w", name, err)
		}
		if err := pending.validateSession(state.SessionID); err != nil {
			return fmt.Errorf("state %s pending steer: %w", name, err)
		}
		pending.command.Input, pending.inputFailure = s.readInput(pending.inputDigest)
		steer = &pending
	}
	s.installSessionState(state.SessionID, state.Draft, state.PendingRuns, state.PendingResume, state.PendingRollback, steer)
	return nil
}

func (s *Store) saveSessionState(sessionID string, draft authoring.Message, pending []PendingRun) error {
	resume, ok := s.pendingResumes[sessionID]
	if !ok {
		return s.saveSessionStateWithResume(sessionID, draft, pending, nil)
	}
	return s.saveSessionStateWithResume(sessionID, draft, pending, &resume)
}

func (s *Store) saveSessionStateWithResume(
	sessionID string,
	draft authoring.Message,
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
	draft authoring.Message,
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
	draft authoring.Message,
	pending []PendingRun,
	resume *PendingResume,
	rollback *PendingSessionRollback,
	steer *PendingSteer,
) error {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	if err := validateSessionDraft(draft); err != nil {
		return err
	}
	if pending, exists := s.sessionDeletions[sessionID]; exists && pending.Phase == SessionDeletionConfirmed {
		return errors.New("session authoring state has been retired")
	}
	name := s.sessionStateName(sessionID)
	if draft.IsEmpty() && len(pending) == 0 && resume == nil && rollback == nil && steer == nil {
		return s.remove(name, func() { s.publishSessionState(sessionID, draft, pending, resume, rollback, steer) })
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
	return s.save(name, state, func() { s.publishSessionState(sessionID, draft, pending, resume, rollback, steer) })
}

// writable reports whether a durable write may proceed. An incomplete
// transaction bars every further write until the Store is reopened, and a Store
// with no persistence at all writes nowhere; both writers ask here so a barred
// Store cannot be barred for one of them and open for the other.
func (s *Store) writable() (bool, error) {
	if s.closed {
		return false, errors.New("workbench is closed")
	}
	if s.writeBarrier != nil {
		return false, s.writeBarrier
	}
	return s.persistence != nil, nil
}

func (s *Store) save(name string, value any, publish func()) error {
	writable, err := s.writable()
	if err != nil {
		return &WriteError{Record: name, Result: commit.Result{Err: err}}
	}
	if !writable {
		publish()
		return nil
	}
	// A state file is read by a person and compared between runs, so the same
	// value must produce the same bytes.
	encoded, err := json.Marshal(
		value,
		jsontext.WithIndent("  "),
		stateJSONOptions,
	)
	if err != nil {
		return fmt.Errorf("encode state snapshot: %w", err)
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maximumStateBytes {
		return fmt.Errorf("workbench state %q exceeds %d bytes", name, maximumStateBytes)
	}
	return s.acceptCommit(name, s.persistence.Replace(name, encoded), publish)
}

func (s *Store) remove(name string, publish func()) error {
	writable, err := s.writable()
	if err != nil {
		return &WriteError{Record: name, Result: commit.Result{Err: err}}
	}
	if !writable {
		publish()
		return nil
	}
	return s.acceptCommit(name, s.persistence.Remove(name), publish)
}

func (s *Store) acceptCommit(name string, result commit.Result, publish func()) error {
	switch result.State {
	case commit.Uncommitted:
		if result.Err == nil {
			return s.blockWrites(errors.New("persistence returned an uncommitted write without a cause"))
		}
		return &WriteError{Record: name, Result: result}
	case commit.Visible, commit.Durable:
		publish()
	default:
		return s.blockWrites(errors.New("persistence returned an unknown commit state"))
	}
	if result.State == commit.Visible {
		if result.Err == nil {
			result.Err = errors.New("persistence did not confirm record durability")
		}
		s.blockWrites(fmt.Errorf("record %q durability is unconfirmed: %w", name, result.Err))
	}
	if result.Err != nil {
		return &WriteError{Record: name, Result: result}
	}
	return nil
}

// DispatchReady fences external effects that depend on this Store's command
// identities. Receipt reads do not require another local publication.
func (s *Store) DispatchReady() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dispatchReadyLocked()
}

func (s *Store) dispatchReadyLocked() error {
	_, err := s.writable()
	if err != nil {
		return fmt.Errorf("%w: command record durability: %w", authoring.ErrCommandOutcomeUnknown, err)
	}
	return nil
}

// CancellationReady checks retained cancellation identities. A live Run can
// still be stopped with an independent, process-owned cancellation intent.
func (s *Store) CancellationReady(commandID authoring.CommandID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, commands := range s.pendingRuns {
		for _, pending := range commands {
			if pending.CancelCommandID == commandID && commandID != "" {
				return s.dispatchReadyLocked()
			}
		}
	}
	return nil
}

func (s *Store) publishSessionState(sessionID string, draft authoring.Message, pending []PendingRun, resume *PendingResume, rollback *PendingSessionRollback, steer *PendingSteer) {
	s.installSessionState(sessionID, draft, pending, resume, rollback, steer)
	// Live publication has the complete outbox catalog. Cold restore trims only
	// after all Sessions have loaded, so another Session's pinned history survives.
	s.history = s.trimHistory(s.history)
}

func (s *Store) installSessionState(
	sessionID string,
	draft authoring.Message,
	pending []PendingRun,
	resume *PendingResume,
	rollback *PendingSessionRollback,
	steer *PendingSteer,
) {
	delete(s.drafts, sessionID)
	if !draft.IsEmpty() {
		s.drafts[sessionID] = draft.Clone()
	}
	delete(s.pendingRuns, sessionID)
	if len(pending) > 0 {
		s.pendingRuns[sessionID] = clonePendingRunSlice(pending)
	}
	delete(s.pendingResumes, sessionID)
	if resume != nil {
		s.pendingResumes[sessionID] = clonePendingResume(*resume)
	}
	delete(s.pendingRollbacks, sessionID)
	if rollback != nil {
		s.pendingRollbacks[sessionID] = rollback.clone()
	}
	delete(s.pendingSteers, sessionID)
	if steer != nil {
		s.pendingSteers[sessionID] = steer.clone()
	}
}

func (s *Store) blockWrites(cause error) error {
	if s.writeBarrier == nil {
		s.writeBarrier = fmt.Errorf(
			"workbench persistence requires reopen after an incomplete transaction: %w",
			cause,
		)
	}
	return errors.Join(cause, s.writeBarrier)
}

func (s *Store) sessionStateName(sessionID string) string {
	digest := sha256.Sum256([]byte(sessionID))
	return filepath.Join("sessions", hex.EncodeToString(digest[:16])+sessionStateExtension)
}
