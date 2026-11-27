package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/filesystem/sessionartifact"
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
	if err := l.root.requireRuntimeFeature(protocol.FeatureSessionExport); err != nil {
		return err
	}
	workspace := authoringDirectory(l.localDirectory, l.state.current.Workspace.Path)
	l.root.message("reading session artifact")
	started := l.tasks.session(sessionOutputOperation,
		func(context.Context) (sessionImport, error) {
			artifact, err := sessionartifact.Store{}.Load(workspace, path)
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
		return sessionOutputOperation.busy()
	}
	return nil
}

func (l *sessionLifecycle) importSession(artifact session.Document) {
	text := sessionChangeText{working: "importing session", committed: "imported session"}
	l.runSessionMutation(text, readCommittedSession, func(ctx context.Context) (conversation.Session, error) {
		return l.transfers.ImportSession(ctx, session.ImportRequest{Artifact: artifact})
	})
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
	started := l.tasks.session(sessionOutputOperation,
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
		return sessionOutputOperation.busy()
	}
	return nil
}

type rollbackPreview struct {
	request    conversation.RollbackTarget
	settlement session.RollbackPreview
}

func previewRollback(snapshot conversation.SessionSnapshot, request conversation.RollbackTarget) (rollbackPreview, error) {
	settlement, err := session.PreviewRollback(snapshot, request)
	return rollbackPreview{request: request, settlement: settlement}, err
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
			if !result.Outcome.Valid() {
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
		l.root.message("rollback refused · " + errors.Join(result.Refusal, settlement.err).Error())
		return nil
	case mutation.Unknown:
		l.root.message("rollback outcome is unknown. The original command is saved for recovery. " + settlement.err.Error())
		return nil
	case mutation.Confirmed:
	default:
		return errors.New("rollback settlement returned an invalid outcome")
	}

	if settlement.err != nil {
		l.root.message("rollback committed. Local recovery is pending. " + settlement.err.Error())
		return nil
	}
	installation, err := l.prepareSessionInstallation(result.Snapshot)
	if err != nil {
		l.root.message("rollback committed. The Session could not be refreshed. " + err.Error())
		return nil
	}
	l.install(installation)
	recovery := installation.rollbackRecovery
	if recovery == nil {
		return errors.New("confirmed rollback did not produce local recovery state")
	}
	l.root.message(describeRollbackDraft("rolled back session", *recovery))
	return nil
}

func rollbackRecoveryNotice(recovery workbench.SessionRollbackRecovery) string {
	return describeRollbackDraft("recovered rollback input", recovery)
}

func describeRollbackDraft(action string, recovery workbench.SessionRollbackRecovery) string {
	label := fmt.Sprintf("%s · %d runs removed", action, recovery.DroppedCount)
	if recovery.Merged {
		label += " · restored opening text before the newer draft"
	}
	if recovery.OpeningImages > 0 {
		label += fmt.Sprintf(" · reattach %d inline images", recovery.OpeningImages)
	}
	return label
}

func parseRollbackArgument(sessionID, argument string) (conversation.RollbackTarget, error) {
	fields := strings.Fields(argument)
	if len(fields) == 0 || len(fields) > 2 {
		return conversation.RollbackTarget{}, errors.New("usage: /rollback <run-id|all> [history|files|both]")
	}
	boundary := fields[0]
	if isAllArgument(boundary) {
		boundary = ""
	}
	scope := protocol.RestoreHistory
	if len(fields) == 2 {
		scope = protocol.RestoreType(strings.ToLower(fields[1]))
	}
	return conversation.RollbackTarget{SessionID: sessionID, ToRunID: boundary, Scope: scope}, nil
}
