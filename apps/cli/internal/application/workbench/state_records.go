package workbench

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	maximumStateBytes   = 16 << 20
	sessionRecordPrefix = "session/"
)

type sessionState struct {
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
	for index, message := range s.history {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("load prompt history entry %d: %w", index+1, err)
		}
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
	s.history = s.history[max(0, len(s.history)-s.historyCapacity):]
	s.stashes = tailStashes(s.stashes, s.stashCapacity)
	s.workspaces = slices.Clone(s.workspaces[:min(len(s.workspaces), s.workspaceCapacity)])
	return nil
}

func (s *Store) load(name string, value any) error {
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

// The codec leaves time.Duration unspecified and the outbox needs an exact unit.
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
	keys, err := s.persistence.Keys(sessionRecordPrefix)
	if err != nil {
		return err
	}
	for _, key := range keys {
		sessionID := strings.TrimPrefix(key, sessionRecordPrefix)
		if err := protocol.ValidateSessionID(sessionID); err != nil {
			return fmt.Errorf("invalid session record %q: %w", key, err)
		}
		var state sessionState
		if err := s.load(key, &state); err != nil {
			return err
		}
		if err := s.restoreSessionState(sessionID, state); err != nil {
			return fmt.Errorf("restore %s: %w", key, err)
		}
	}
	return nil
}

func (s *Store) restoreSessionState(sessionID string, state sessionState) error {
	if err := validateSessionDraft(state.Draft); err != nil {
		return err
	}
	if err := validatePendingRunSequence(sessionID, state.PendingRuns); err != nil {
		return err
	}
	for index := range state.PendingRuns {
		pending := &state.PendingRuns[index]
		if pending.Dispatch != nil {
			key := inputRecordKey(sessionID, startInput, pending.Command.CommandID)
			pending.Command.Input, pending.Dispatch.inputFailure = s.readInput(key, pending.Dispatch.InputDigest)
		}
	}
	if state.PendingResume != nil {
		if err := state.PendingResume.validate(); err != nil {
			return fmt.Errorf("pending resume: %w", err)
		}
		key := inputRecordKey(sessionID, resumeInput, state.PendingResume.Command.CommandID)
		state.PendingResume.Command.Input, state.PendingResume.inputFailure = s.readInput(key, state.PendingResume.InputDigest)
	}
	if state.PendingRollback != nil {
		if err := state.PendingRollback.Validate(); err != nil {
			return fmt.Errorf("pending rollback: %w", err)
		}
		if state.PendingRollback.SessionID != sessionID {
			return errors.New("pending rollback belongs to another session")
		}
	}
	var steer *PendingSteer
	if state.PendingSteer != nil {
		pending, err := restorePendingSteer(*state.PendingSteer)
		if err != nil {
			return fmt.Errorf("pending steer: %w", err)
		}
		if err := pending.validateSession(sessionID); err != nil {
			return fmt.Errorf("pending steer: %w", err)
		}
		key := inputRecordKey(sessionID, steerInput, pending.command.CommandID)
		pending.command.Input, pending.inputFailure = s.readInput(key, pending.inputDigest)
		steer = &pending
	}
	s.installSessionState(sessionID, state.Draft, state.PendingRuns, state.PendingResume, state.PendingRollback, steer)
	return nil
}

func (s *Store) saveSessionState(sessionID string, draft authoring.Message, pending []PendingRun) error {
	return s.saveSessionStateWithResume(sessionID, draft, pending, s.pendingResumePointer(sessionID))
}

func (s *Store) saveSessionStateWithResume(
	sessionID string,
	draft authoring.Message,
	pending []PendingRun,
	resume *PendingResume,
) error {
	return s.saveSessionStateRecord(sessionID, draft, pending, resume, s.pendingRollbackPointer(sessionID), s.pendingSteerPointer(sessionID))
}

func (s *Store) saveSessionStateRecord(
	sessionID string,
	draft authoring.Message,
	pending []PendingRun,
	resume *PendingResume,
	rollback *PendingSessionRollback,
	steer *PendingSteer,
) error {
	update := sessionUpdate{sessionID: sessionID, draft: draft, runs: pending, resume: resume, rollback: rollback, steer: steer}
	return s.commitSessions([]sessionUpdate{update})
}

type sessionUpdate struct {
	sessionID string
	draft     authoring.Message
	runs      []PendingRun
	resume    *PendingResume
	rollback  *PendingSessionRollback
	steer     *PendingSteer
}

func (s *Store) sessionUpdate(sessionID string) sessionUpdate {
	return sessionUpdate{
		sessionID: sessionID, draft: s.drafts[sessionID], runs: s.queue.bySession[sessionID],
		resume: s.pendingResumePointer(sessionID), rollback: s.pendingRollbackPointer(sessionID), steer: s.pendingSteerPointer(sessionID),
	}
}

func (u sessionUpdate) record() (RecordChange, error) {
	if err := protocol.ValidateSessionID(u.sessionID); err != nil {
		return RecordChange{}, err
	}
	if err := validateSessionDraft(u.draft); err != nil {
		return RecordChange{}, err
	}
	key := sessionStateKey(u.sessionID)
	if u.draft.IsEmpty() && len(u.runs) == 0 && u.resume == nil && u.rollback == nil && u.steer == nil {
		return RecordChange{Key: key}, nil
	}
	state := sessionState{Draft: u.draft, PendingRuns: u.runs, PendingResume: u.resume, PendingRollback: u.rollback}
	if u.steer != nil {
		record := u.steer.record()
		state.PendingSteer = &record
	}
	return encodeRecord(key, state)
}

func (s *Store) commitSessions(updates []sessionUpdate, changes ...RecordChange) error {
	for _, update := range updates {
		record, err := update.record()
		if err != nil {
			return err
		}
		changes = append(changes, record)
		keptInputs := update.inputKeys()
		for _, key := range s.sessionUpdate(update.sessionID).inputKeys() {
			if !slices.Contains(keptInputs, key) {
				changes = append(changes, RecordChange{Key: key})
			}
		}
	}
	if err := s.commit(changes...); err != nil {
		return err
	}
	for _, update := range updates {
		s.installSessionState(update.sessionID, update.draft, update.runs, update.resume, update.rollback, update.steer)
	}
	return nil
}

func (u sessionUpdate) inputKeys() []string {
	var keys []string
	if len(u.runs) != 0 {
		head := u.runs[0]
		if head.Dispatch != nil && head.Dispatch.InputDigest != "" {
			keys = append(keys, inputRecordKey(u.sessionID, startInput, head.Command.CommandID))
		}
	}
	if u.resume != nil && u.resume.InputDigest != "" {
		keys = append(keys, inputRecordKey(u.sessionID, resumeInput, u.resume.Command.CommandID))
	}
	if u.steer != nil && u.steer.inputDigest != "" {
		keys = append(keys, inputRecordKey(u.sessionID, steerInput, u.steer.command.CommandID))
	}
	return keys
}

func (s *Store) admitWrite() error {
	if s.closed {
		return errors.New("workbench is closed")
	}
	return nil
}

func encodeRecord(key string, value any) (RecordChange, error) {
	encoded, err := json.Marshal(value, stateJSONOptions)
	if err != nil {
		return RecordChange{}, fmt.Errorf("encode workbench record %q: %w", key, err)
	}
	if len(encoded) > maximumStateBytes {
		return RecordChange{}, fmt.Errorf("workbench record %q exceeds %d bytes", key, maximumStateBytes)
	}
	return RecordChange{Key: key, Body: encoded}, nil
}

func (s *Store) commit(changes ...RecordChange) error {
	if err := s.admitWrite(); err != nil {
		return err
	}
	if err := s.persistence.Commit(changes...); err != nil {
		return fmt.Errorf("commit workbench state: %w", err)
	}
	return nil
}

func (s *Store) save(key string, value any, publish func()) error {
	record, err := encodeRecord(key, value)
	if err != nil {
		return err
	}
	if err := s.commit(record); err != nil {
		return err
	}
	publish()
	return nil
}

func (s *Store) DispatchReady() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dispatchReadyLocked()
}

func (s *Store) dispatchReadyLocked() error {
	if err := s.admitWrite(); err != nil {
		return fmt.Errorf("%w: command record durability: %w", authoring.ErrCommandOutcomeUnknown, err)
	}
	return nil
}

func (s *Store) CancellationReady(commandID authoring.CommandID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, commands := range s.queue.bySession {
		for _, pending := range commands {
			if cancellation := pending.Cancellation(); cancellation != nil && cancellation.CommandID == commandID {
				return s.dispatchReadyLocked()
			}
		}
	}
	return nil
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
	s.queue.install(sessionID, pending)
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

func sessionStateKey(sessionID string) string { return sessionRecordPrefix + sessionID }
