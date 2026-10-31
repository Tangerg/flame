package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type sessionImport struct {
	path     string
	artifact session.Document
}

func (l *sessionLifecycle) prepareSessionImport(path string) error {
	if l.transfers == nil {
		return errors.New("this runtime composition has no session transfer service")
	}
	if err := l.root.requireRuntimeFeature(protocol.FeatureSessionExport); err != nil {
		return err
	}
	workspace := authoringDirectory(l.localDirectory, l.state.current.Workspace.Path)
	l.root.message("reading session artifact")
	started := l.tasks.session(sessionOutputOperation, false,
		func(context.Context) (sessionImport, error) {
			artifact, err := l.artifacts.Load(workspace, path)
			return sessionImport{path: path, artifact: artifact}, err
		},
		func(prepared sessionImport, err error) {
			if err != nil {
				l.root.message("import session failed: " + err.Error())
				return
			}
			l.confirm.ask(
				"Import session",
				"Import "+prepared.path+"? An existing session with the artifact ID will be replaced.",
				"Import and replace",
				"",
				func() { l.importSession(prepared.artifact) },
			)
		},
	)
	if !started {
		return errors.New("another session output operation is already running")
	}
	return nil
}

func (l *sessionLifecycle) importSession(artifact session.Document) {
	l.runSessionChange("importing session",
		func(ctx context.Context) (conversation.SessionSnapshot, error) {
			imported, err := l.transfers.ImportSession(ctx, session.ImportRequest{Artifact: artifact})
			if err != nil {
				return conversation.SessionSnapshot{}, err
			}
			return l.readSessionAfterMutation(ctx, imported.ID)
		},
		func(snapshot conversation.SessionSnapshot) error {
			if err := l.installSnapshot(snapshot); err != nil {
				return err
			}
			l.root.message("imported session · " + displayTitle(snapshot.Session))
			return nil
		},
	)
}

func (l *sessionLifecycle) prepareSessionRollback(argument string) error {
	request, err := parseRollbackArgument(l.state.current.ID, argument)
	if err != nil {
		return err
	}
	if request.RestoresFiles() {
		if err := l.root.requireRuntimeFeature(protocol.FeatureCheckpoints); err != nil {
			return err
		}
	}
	l.root.message("previewing rollback")
	started := l.tasks.session(sessionOutputOperation, false,
		func(ctx context.Context) (rollbackPreview, error) {
			snapshot, err := l.runtime.GetSession(ctx, request.SessionID)
			if err != nil {
				return rollbackPreview{}, err
			}
			return previewRollback(snapshot, request)
		},
		func(preview rollbackPreview, err error) {
			if err != nil {
				l.root.message("preview rollback failed: " + err.Error())
				return
			}
			l.confirm.ask(
				"Rollback session",
				preview.Description()+" This cannot be undone.",
				"Rollback permanently",
				"",
				func() { l.rollbackSession(preview) },
			)
		},
	)
	if !started {
		return errors.New("another session output operation is already running")
	}
	return nil
}

type rollbackPreview struct {
	request    conversation.RollbackSession
	settlement session.RollbackPreview
}

func previewRollback(snapshot conversation.SessionSnapshot, request conversation.RollbackSession) (rollbackPreview, error) {
	settlement, err := session.PreviewRollback(snapshot, request)
	return rollbackPreview{request: request, settlement: settlement}, err
}

func (r rollbackPreview) ValidateCommit(snapshot conversation.SessionSnapshot) error {
	return r.settlement.ValidateCommit(snapshot)
}

// ValidateApplied proves a history rollback committed when its command result
// was lost behind a post-commit cleanup error. Files-only rollback has no
// observable session projection change and therefore cannot use this proof.
func (r rollbackPreview) ValidateApplied(snapshot conversation.SessionSnapshot) error {
	return r.settlement.ValidateApplied(snapshot)
}

func (r rollbackPreview) Description() string {
	boundary := "the empty-session boundary"
	if r.request.ToRunID != "" {
		boundary = shortIdentity(r.request.ToRunID)
	}
	if r.request.FilesOnly() {
		return fmt.Sprintf("Restore checkpointed files to %s while keeping chat history? Ignored and oversized files are not archived; conflicting unarchived content blocks restore.", boundary)
	}
	if r.request.RestoresFiles() {
		return fmt.Sprintf("Restore checkpointed files to %s, then remove %d later runs? Ignored and oversized files are not archived; conflicting unarchived content blocks restore. Partial failures retain recovery intent.", boundary, r.settlement.DroppedCount())
	}
	return fmt.Sprintf("Restore %s to %s and remove %d later runs?", r.request.Scope, boundary, r.settlement.DroppedCount())
}

type rollbackSettlement struct {
	result session.RollbackResult
	err    error
}

func (l *sessionLifecycle) rollbackSession(preview rollbackPreview) {
	l.runSessionChange("rolling back session",
		func(ctx context.Context) (rollbackSettlement, error) {
			result, err := session.Rollback(
				ctx, l.runtime, l.workbench, preview.settlement,
				l.replayPolicy, runtimeRecoveryBackoff,
			)
			if result.Pending.CommandID == "" {
				return rollbackSettlement{}, err
			}
			return rollbackSettlement{result: result, err: err}, nil
		},
		func(settlement rollbackSettlement) error {
			return l.applyRollbackSettlement(settlement)
		},
	)
}

func (l *sessionLifecycle) applyRollbackSettlement(settlement rollbackSettlement) error {
	result := settlement.result
	switch result.Outcome {
	case mutation.Rejected:
		if err := session.RejectRollback(l.workbench, result); err != nil {
			l.root.message("rollback session failed; local intent cleanup failed: " + errors.Join(settlement.err, err).Error())
			return nil
		}
		l.root.message("rollback session failed: " + settlement.err.Error())
		return nil
	case mutation.Unknown:
		l.root.message("rollback outcome is unknown; the original request is saved for recovery; inspect the session before issuing a new command: " + settlement.err.Error())
		return nil
	case mutation.Confirmed:
	default:
		return errors.New("rollback settlement returned an invalid outcome")
	}

	confirmationErr := session.ConfirmRollback(l.workbench, result)
	installation, err := l.prepareSessionInstallation(result.Snapshot)
	if err != nil {
		return errors.Join(confirmationErr, err)
	}
	l.install(installation)
	recovery := installation.rollbackRecovery
	if confirmationErr != nil {
		current, _, draftErr := l.composer.draft()
		if draftErr != nil {
			return errors.Join(confirmationErr, draftErr)
		}
		recovered, merged := workbench.MergeSessionRollbackDraft(current, result.Pending)
		recovery = &workbench.SessionRollbackRecovery{
			Draft: recovered, DroppedCount: len(result.Pending.BeforeRunIDs) - len(result.Pending.AfterRunIDs),
			OpeningImages: result.Pending.OpeningImages, Merged: merged,
		}
		if persistErr := l.composer.recoverDraft(recovered); persistErr != nil {
			label := fmt.Sprintf("rolled back %d runs; restored text was not saved: %v", recovery.DroppedCount, persistErr)
			label = appendRollbackWarnings(label, recovery.OpeningImages, settlement.err, confirmationErr)
			l.root.message(label)
			return nil
		}
	}
	if recovery == nil {
		return errors.New("confirmed rollback did not produce local recovery state")
	}
	label := fmt.Sprintf("rolled back session · %d runs removed", recovery.DroppedCount)
	if recovery.Merged {
		label += "; restored opening text before the newer draft"
	}
	label = appendRollbackWarnings(label, recovery.OpeningImages, settlement.err, confirmationErr)
	l.root.message(label)
	return nil
}

func appendRollbackWarnings(label string, images int, cleanupErr, journalErr error) string {
	if images > 0 {
		label += fmt.Sprintf("; %d inline images must be reattached", images)
	}
	if cleanupErr != nil {
		label += "; runtime cleanup warning: " + cleanupErr.Error()
	}
	if journalErr != nil {
		label += "; local recovery journal warning: " + journalErr.Error()
	}
	return label
}

func rollbackRecoveryNotice(recovery workbench.SessionRollbackRecovery) string {
	label := fmt.Sprintf("recovered rollback input · %d runs removed", recovery.DroppedCount)
	if recovery.Merged {
		label += "; restored opening text before the newer draft"
	}
	if recovery.OpeningImages > 0 {
		label += fmt.Sprintf("; %d inline images must be reattached", recovery.OpeningImages)
	}
	return label
}

func parseRollbackArgument(sessionID, argument string) (conversation.RollbackSession, error) {
	fields := strings.Fields(argument)
	if len(fields) == 0 || len(fields) > 2 {
		return conversation.RollbackSession{}, errors.New("usage: /rollback <run-id|all> [history|files|both]")
	}
	boundary := fields[0]
	if strings.EqualFold(boundary, "all") {
		boundary = ""
	}
	scope := protocol.RestoreHistory
	if len(fields) == 2 {
		scope = protocol.RestoreType(strings.ToLower(fields[1]))
	}
	request := conversation.RollbackSession{SessionID: sessionID, ToRunID: boundary, Scope: scope}
	if err := request.Validate(); err != nil {
		return conversation.RollbackSession{}, err
	}
	return request, nil
}
