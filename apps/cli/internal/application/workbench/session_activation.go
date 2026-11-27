package workbench

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type SessionActivation struct {
	SessionID     string
	WorkspacePath string
	InitialDraft  authoring.Message
	source        *sessionActivationSource
}

type sessionActivationSource struct {
	sessionID string
	remaining authoring.Message
	retired   bool
}

func (a SessionActivation) TransferringDraftFrom(sessionID string, remaining authoring.Message) SessionActivation {
	a.source = &sessionActivationSource{sessionID: sessionID, remaining: remaining.Clone()}
	return a
}

func (a SessionActivation) ReplacingDeletedSession(sessionID string) SessionActivation {
	a.source = &sessionActivationSource{sessionID: sessionID, retired: true}
	return a
}

type ActivatedSession struct {
	Draft            authoring.Message
	Rollback         *SessionRollbackRecovery
	DiscardedPrompts int
}

func (s *Store) ActivateSession(input SessionActivation) (ActivatedSession, error) {
	if err := protocol.ValidateSessionID(input.SessionID); err != nil {
		return ActivatedSession{}, err
	}
	source := input.source
	if source != nil {
		if err := protocol.ValidateSessionID(source.sessionID); err != nil {
			return ActivatedSession{}, fmt.Errorf("session activation source: %w", err)
		}
		if source.sessionID == input.SessionID {
			return ActivatedSession{}, errors.New("session activation source and destination are the same session")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, pending := s.sessionDeletions[input.SessionID]; pending {
		return ActivatedSession{}, errors.New("session deletion acknowledgement is still pending")
	}
	workspace := Workspace{Path: input.WorkspacePath, LastOpened: time.Now().UTC()}
	if err := workspace.Validate(); err != nil {
		return ActivatedSession{}, err
	}
	workspaces := slices.DeleteFunc(slices.Clone(s.workspaces), func(item Workspace) bool { return item.Path == workspace.Path })
	workspaces = slices.Insert(workspaces, 0, workspace)
	workspaces = workspaces[:min(len(workspaces), maxRecentWorkspaces)]
	workspaceRecord, err := encodeRecord(workspacesName, workspaces)
	if err != nil {
		return ActivatedSession{}, err
	}
	records := []RecordChange{workspaceRecord}
	destination := s.sessionUpdate(input.SessionID)
	previousDraft := destination.draft
	var updates []sessionUpdate
	var deletions map[string]PendingSessionDeletion
	var activated ActivatedSession
	if source != nil {
		origin := s.sessionUpdate(source.sessionID)
		if !origin.draft.Equal(source.remaining) {
			destination.draft, err = mergeSessionDraft(destination.draft, origin.draft)
			if err != nil {
				return ActivatedSession{}, fmt.Errorf("merge transferred draft: %w", err)
			}
			origin.draft = source.remaining
			if !source.retired {
				updates = append(updates, origin)
			}
		}
		if source.retired {
			activated.DiscardedPrompts = len(origin.runs)
			updates = append(updates, sessionUpdate{sessionID: source.sessionID})
			if _, recorded := s.sessionDeletions[source.sessionID]; recorded {
				var record RecordChange
				deletions, record, err = s.withoutSessionDeletion(source.sessionID)
				if err != nil {
					return ActivatedSession{}, err
				}
				records = append(records, record)
			}
		}
	}
	destination.draft, err = mergeSessionDraft(destination.draft, input.InitialDraft)
	if err != nil {
		return ActivatedSession{}, err
	}
	activated.Rollback, err = destination.recoverRollbackDraft()
	if err != nil {
		return ActivatedSession{}, err
	}
	if !previousDraft.Equal(destination.draft) || activated.Rollback != nil {
		updates = append(updates, destination)
	}
	if err := s.commitSessions(updates, records...); err != nil {
		return ActivatedSession{}, err
	}
	s.workspaces = workspaces
	if deletions != nil {
		s.sessionDeletions = deletions
	}
	activated.Draft = destination.draft.Clone()
	return activated, nil
}

func (u *sessionUpdate) recoverRollbackDraft() (*SessionRollbackRecovery, error) {
	if u.rollback == nil || u.rollback.Restoration == nil {
		return nil, nil
	}
	rollback := u.rollback.Restoration
	merged, err := mergeSessionDraft(authoring.Message{Text: rollback.OpeningText}, u.draft)
	if err != nil {
		return nil, err
	}
	recovery := &SessionRollbackRecovery{
		DroppedCount:  rollback.DroppedCount,
		OpeningImages: rollback.OpeningImages,
		Merged:        !u.draft.IsEmpty() && !u.draft.Equal(merged),
	}
	u.draft, u.rollback = merged, nil
	return recovery, nil
}

func mergeSessionDraft(existing, incoming authoring.Message) (authoring.Message, error) {
	values := []struct {
		label   string
		message authoring.Message
	}{
		{label: "existing", message: existing},
		{label: "incoming", message: incoming},
	}
	for _, value := range values {
		if value.message.IsEmpty() {
			continue
		}
		if err := value.message.Validate(); err != nil {
			return authoring.Message{}, fmt.Errorf("%s session draft: %w", value.label, err)
		}
	}
	if existing.IsEmpty() {
		return incoming.Clone(), nil
	}
	if incoming.IsEmpty() {
		return existing.Clone(), nil
	}

	merged := existing.Clone()
	switch {
	case strings.TrimSpace(merged.Text) == "":
		merged.Text = incoming.Text
	case strings.TrimSpace(incoming.Text) == "", merged.Text == incoming.Text:
	default:
		merged.Text += "\n\n" + incoming.Text
	}
	byID := make(map[string]authoring.Attachment, len(merged.Attachments)+len(incoming.Attachments))
	byPath := make(map[string]authoring.Attachment, len(merged.Attachments)+len(incoming.Attachments))
	for _, item := range merged.Attachments {
		byID[item.ID] = item
		byPath[item.Path] = item
	}
	for _, item := range incoming.Attachments {
		if current, found := byID[item.ID]; found {
			if current != item {
				return authoring.Message{}, fmt.Errorf("session drafts conflict on attachment id %q", item.ID)
			}
			continue
		}
		if current, found := byPath[item.Path]; found {
			if current != item {
				return authoring.Message{}, fmt.Errorf("session drafts conflict on attachment path %q", item.Path)
			}
			continue
		}
		merged.Attachments = append(merged.Attachments, item)
		byID[item.ID] = item
		byPath[item.Path] = item
	}
	if err := merged.Validate(); err != nil {
		return authoring.Message{}, fmt.Errorf("merged session draft: %w", err)
	}
	return merged, nil
}
