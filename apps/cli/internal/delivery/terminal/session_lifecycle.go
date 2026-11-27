package terminal

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/filesystem/attachment"
	"github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/retry"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

type sessionRoot interface {
	message(label string)
	presentSession(session conversation.Session)
	displayOptions() authoring.RunOptions
	currentTranscript() *transcriptView
	newTranscript() *transcriptView
	presentTranscript(transcript *transcriptView) *transcriptView
	enterSessionHealth()
	dismissModals()
	dismissCompletion()
	cancelPluginCommands(pluginIDs ...string)
	followRuntimeChanges()
	requireRuntimeFeature(feature string) error
}

type sessionLifecycle struct {
	state sessionState
	root  sessionRoot

	runtime        Runtime
	workspaces     Workspaces
	transfers      session.TransferService
	workbench      *workbench.Store
	queue          *workbench.Queue
	replayPolicy   mutation.ReplayPolicy
	localDirectory string
	tasks          taskRunner
	report         func(workbenchConcern, error)

	status   *statusView
	prompt   *promptView
	header   *sessionHeader
	activity *activityView

	runs             *runs
	composer         *promptComposer
	prompts          *interruptPrompts
	steering         *steerDesk
	reader           *readerSurface
	confirm          *confirmPrompt
	sessions         *sessionBrowser
	workspaceChoices *pickerDialog[workspaceChoice]
}

func (l *sessionLifecycle) NewSession() {
	l.startSessionInWorkspace(l.state.current.Workspace.Path)
}

func (l *sessionLifecycle) RenameSession(title string) {
	if l.runs.observing() {
		l.root.message("finish or cancel the current run before renaming the session")
		return
	}
	title = strings.TrimSpace(title)
	if title == "" {
		l.root.message("/rename needs a non-empty title")
		return
	}
	target := l.state.current
	l.runSessionChange("renaming session",
		func(ctx context.Context) (conversation.Session, error) {
			return l.runtime.UpdateSession(ctx, conversation.UpdateSession{
				SessionID: target.ID, Title: &title, ExpectedRevision: target.Revision,
			})
		},
		func(updated conversation.Session) error {
			l.setActiveSession(updated)
			l.root.message("renamed session to " + updated.Title)
			return nil
		},
	)
}

func (l *sessionLifecycle) ForkSession(title string) {
	l.forkSession(conversation.ForkSession{SessionID: l.state.current.ID, Title: strings.TrimSpace(title)})
}

func (l *sessionLifecycle) forkSessionFromRun(runID string) {
	l.forkSession(conversation.ForkSession{
		SessionID: l.state.current.ID, FromRunID: runID, Title: "Fork from " + shortIdentity(runID),
	})
}

func (l *sessionLifecycle) forkSession(request conversation.ForkSession) {
	text := sessionChangeText{working: "forking session", committed: "forked session"}
	if request.FromRunID != "" {
		text.working += " from " + shortIdentity(request.FromRunID)
	}
	l.runSessionMutation(text, readCommittedSession, func(ctx context.Context) (conversation.Session, error) {
		return l.runtime.ForkSession(ctx, request)
	})
}

func (l *sessionLifecycle) switchSession(id string) {
	if id == l.state.current.ID && l.state.freshness != sessionMissing {
		l.root.message("already in " + displayTitle(l.state.current))
		return
	}
	l.runSessionChange("loading session",
		func(ctx context.Context) (conversation.SessionSnapshot, error) { return l.runtime.GetSession(ctx, id) },
		func(snapshot conversation.SessionSnapshot) error { return l.installSnapshot(snapshot) },
	)
}

type sessionInstallation struct {
	snapshot         conversation.SessionSnapshot
	attachments      *attachment.Resolver
	projection       sessionProjection
	draft            authoring.Message
	rollbackRecovery *workbench.SessionRollbackRecovery
}

func (l *sessionLifecycle) prepareSessionInstallation(snapshot conversation.SessionSnapshot) (sessionInstallation, error) {
	attachments, err := attachment.New(authoringDirectory(l.localDirectory, snapshot.Session.Workspace.Path))
	if err != nil {
		return sessionInstallation{}, fmt.Errorf("session attachments: %w", err)
	}
	projection, err := l.projectSession(snapshot, nil)
	if err != nil {
		return sessionInstallation{}, fmt.Errorf("install session: %w", err)
	}
	draft, recovery, err := l.prepareDestinationDraft(snapshot.Session)
	if err != nil {
		projection.close()
		return sessionInstallation{}, err
	}
	return sessionInstallation{
		snapshot: snapshot, attachments: attachments, projection: projection, draft: draft,
		rollbackRecovery: recovery,
	}, nil
}

func (l *sessionLifecycle) prepareDestinationDraft(
	session conversation.Session,
) (authoring.Message, *workbench.SessionRollbackRecovery, error) {
	current, _, err := l.composer.draft()
	if err != nil {
		return authoring.Message{}, nil, err
	}
	if saveDraftErr := l.composer.saveDraft(current); saveDraftErr != nil {
		return authoring.Message{}, nil, fmt.Errorf("save source session draft: %w", saveDraftErr)
	}
	if err := l.workbench.CheckSessionActivation(session.ID); err != nil {
		return authoring.Message{}, nil, fmt.Errorf("activate destination session state: %w", err)
	}
	if rememberWorkspaceErr := l.workbench.RememberWorkspace(session.Workspace.Path); rememberWorkspaceErr != nil {
		return authoring.Message{}, nil, fmt.Errorf("remember workspace: %w", rememberWorkspaceErr)
	}
	if change := l.state.change; change != nil && session.ID != l.state.current.ID {
		remaining := change.baseline
		if l.state.freshness == sessionMissing {
			remaining = authoring.Message{}
		}
		if err := l.workbench.TransferSessionDraft(l.state.current.ID, session.ID, remaining); err != nil {
			return authoring.Message{}, nil, fmt.Errorf("transfer session draft: %w", err)
		}
	}
	activation, err := l.workbench.ActivateSessionDraft(session.ID, authoring.Message{})
	if err != nil {
		return authoring.Message{}, nil, fmt.Errorf("activate destination session draft: %w", err)
	}
	return activation.Draft, activation.Rollback, nil
}

// A replaced session is already installed, so its local cleanup is reported rather than failing the replacement.
func (l *sessionLifecycle) reportRetiredSessionState(discarded int, err error) {
	if err != nil {
		l.root.message("replacement installed; local state cleanup failed: " + err.Error())
		return
	}
	if discarded > 0 {
		l.root.message(fmt.Sprintf("discarded %s from the deleted session", countedNoun(discarded, "queued prompt")))
	}
}

func (l *sessionLifecycle) retireSessionState(sessionID string) (int, error) {
	before := len(l.queue.Snapshot(sessionID).Entries)
	err := l.workbench.RetireSessionState(sessionID)
	discarded := before - len(l.queue.Snapshot(sessionID).Entries)
	if err != nil {
		return discarded, fmt.Errorf("discard session authoring state: %w", err)
	}
	return discarded, nil
}

func (l *sessionLifecycle) readSessionAfterMutation(ctx context.Context, sessionID string) (conversation.SessionSnapshot, error) {
	for failures := 0; ; {
		snapshot, err := l.runtime.GetSession(ctx, sessionID)
		if err == nil {
			return snapshot, nil
		}
		failures++
		delay, shouldRetry, policyErr := retry.ReconnectDelay(failures, err)
		if policyErr != nil {
			return conversation.SessionSnapshot{}, policyErr
		}
		if !shouldRetry {
			return conversation.SessionSnapshot{}, err
		}
		if err := retry.Wait(ctx, delay); err != nil {
			return conversation.SessionSnapshot{}, err
		}
	}
}

func (l *sessionLifecycle) install(s sessionInstallation) {
	previousSessionID := l.state.current.ID
	previousWorkspace := l.state.current.Workspace
	previousMissing := l.state.freshness == sessionMissing
	switching := previousSessionID != s.snapshot.Session.ID
	if switching || previousMissing {
		l.state.freshness = sessionObserved
	}
	l.prepareSessionProjectionReplacement(s.snapshot.Session, s.projection.conversation)
	l.root.cancelPluginCommands()
	l.tasks.operations.CancelScope(sessionOperationScope)
	l.runs.dropStream()
	l.root.dismissCompletion()
	l.setActiveSession(s.snapshot.Session)
	l.queue.ReleasePreparation(previousSessionID)
	l.runs.switchProjection(s.projection.conversation)
	l.composer.attachments = s.attachments
	previousTranscript := l.root.presentTranscript(s.projection.transcript)
	l.composer.restoreComposer(s.draft)
	l.composer.drafts.adopt(l.state.current.ID, s.draft)
	l.activity.Reset()
	l.status.Reset()
	l.root.enterSessionHealth()
	l.header.SetUsage(s.projection.conversation.Usage())
	l.prompt.SetOptions(l.root.displayOptions())
	l.prompt.SetBusy(s.projection.conversation.Busy())
	l.runs.syncQueue()
	previousTranscript.Close()
	l.restoreActivity(s.snapshot)
	l.runs.restoreSessionOutbox()
	if l.state.current.Workspace != previousWorkspace {
		l.root.followRuntimeChanges()
	}
	if l.runs.conversation().Phase() == conversation.Idle {
		l.root.message("session · " + displayTitle(s.snapshot.Session))
		if l.state.freshness == sessionStale {
			l.refreshInvalidatedSession(refreshAfterChange)
		}
	}
	l.steering.restoreSteerReceipts(s.snapshot)
	if previousMissing && switching {
		l.reportRetiredSessionState(l.retireSessionState(previousSessionID))
	}
}

func (l *sessionLifecycle) installSnapshot(snapshot conversation.SessionSnapshot) error {
	installation, err := l.prepareSessionInstallation(snapshot)
	if err != nil {
		return err
	}
	l.install(installation)
	if installation.rollbackRecovery != nil {
		l.root.message(rollbackRecoveryNotice(*installation.rollbackRecovery))
	}
	return nil
}
