package terminal

import (
	"context"
	"errors"
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
	source := l.state.current.ID
	l.runSessionChange("forking session",
		func(ctx context.Context) (conversation.SessionSnapshot, error) {
			forked, err := l.runtime.ForkSession(ctx, conversation.ForkSession{SessionID: source, Title: strings.TrimSpace(title)})
			if err != nil {
				return conversation.SessionSnapshot{}, err
			}
			return l.readSessionAfterMutation(ctx, forked.ID)
		},
		func(snapshot conversation.SessionSnapshot) error { return l.installSnapshot(snapshot) },
	)
}

func (l *sessionLifecycle) forkSessionFromRun(runID string) {
	source := l.state.current.ID
	short := shortIdentity(runID)
	l.runSessionChange("forking session from "+short,
		func(ctx context.Context) (conversation.SessionSnapshot, error) {
			forked, err := l.runtime.ForkSession(ctx, conversation.ForkSession{
				SessionID: source, FromRunID: runID, Title: "Fork from " + short,
			})
			if err != nil {
				return conversation.SessionSnapshot{}, err
			}
			return l.readSessionAfterMutation(ctx, forked.ID)
		},
		func(snapshot conversation.SessionSnapshot) error { return l.installSnapshot(snapshot) },
	)
}

func (l *sessionLifecycle) switchSession(id string) {
	if id == l.state.current.ID {
		l.root.message("already in " + displayTitle(l.state.current))
		return
	}
	l.runSessionChange("loading session",
		func(ctx context.Context) (conversation.SessionSnapshot, error) { return l.runtime.GetSession(ctx, id) },
		func(snapshot conversation.SessionSnapshot) error { return l.installSnapshot(snapshot) },
	)
}

func (l *sessionLifecycle) runSessionChange[T any](label string, work func(context.Context) (T, error), apply func(T) error) {
	l.runSessionChangeWithDraftDisposition(label, preserveSourceDraft, work, apply)
}

func (l *sessionLifecycle) runSessionChangeWithDraftDisposition[T any](
	label string,
	disposition sourceDraftDisposition,
	work func(context.Context) (T, error),
	apply func(T) error,
) {
	if l.runs.observing() {
		l.root.message("finish or cancel the current run before changing sessions")
		return
	}
	if l.runs.cancelPending() {
		l.root.message("wait for runtime cancellation to finish")
		return
	}
	if l.tasks.operations.Active(sessionChangeOperation) {
		l.root.message(sessionChangeOperation.busy().Error())
		return
	}
	if l.tasks.operations.Active(sessionCenterOperation) {
		l.root.message(sessionCenterOperation.busy().Error())
		return
	}
	l.tasks.operations.Cancel(pickerCatalogOperation)
	l.sessions.dialog.Dismiss()
	baseline, _, err := l.composer.draft()
	if err != nil {
		l.root.message(label + " failed: " + err.Error())
		return
	}
	if err := l.composer.saveDraft(baseline); err != nil {
		l.report(workbenchDraft, err)
		l.root.message("session change blocked: save session draft: " + err.Error())
		return
	}
	l.report(workbenchDraft, nil)
	l.state.draftTransition = &sessionDraftTransition{
		sourceSessionID: l.state.current.ID,
		baseline:        baseline,
		disposition:     disposition,
	}
	l.root.message(label)
	if !l.tasks.session(sessionChangeOperation, work, func(result T, err error) {
		defer l.settleSessionChange()
		if err != nil {
			l.root.message(label + " failed: " + err.Error())
			return
		}
		if err := apply(result); err != nil {
			l.root.message(label + " failed: " + err.Error())
		}
	}) {
		l.settleSessionChange()
		l.root.message(sessionChangeOperation.busy().Error())
	}
}

func (l *sessionLifecycle) cancelSessionChange() bool {
	if !l.tasks.operations.Active(sessionChangeOperation) {
		return false
	}
	l.tasks.operations.Cancel(sessionChangeOperation)
	l.root.message("session change canceled")
	l.settleSessionChange()
	return true
}

func (l *sessionLifecycle) settleSessionChange() {
	l.state.draftTransition = nil
	if l.state.invalidated && l.runs.conversation().Phase() != conversation.Running &&
		!l.runs.following() && !l.runs.cancelPending() {
		l.refreshInvalidatedSession(refreshAfterChange)
	}
}

type sourceDraftDisposition uint8

const (
	preserveSourceDraft sourceDraftDisposition = iota
	retireSourceDraft
)

type sessionDraftTransition struct {
	sourceSessionID string
	baseline        authoring.Message
	disposition     sourceDraftDisposition
}

func (s sessionDraftTransition) resolve(
	store *workbench.Store,
	destinationSessionID string,
	destinationDraft authoring.Message,
	currentDraft authoring.Message,
) (authoring.Message, error) {
	switch s.disposition {
	case retireSourceDraft:
		if destinationSessionID == s.sourceSessionID {
			return authoring.Message{}, fmt.Errorf("replacement session reused retired identity %s", destinationSessionID)
		}
		merged, err := workbench.MergeSessionDraft(destinationDraft, currentDraft)
		if err != nil {
			return authoring.Message{}, fmt.Errorf("merge replacement session draft: %w", err)
		}
		if strings.TrimSpace(currentDraft.Text) == "" && len(currentDraft.Attachments) == 0 {
			return merged, nil
		}
		if err := store.ApplyDraftTransfer(workbench.DraftTransfer{
			SourceSessionID: s.sourceSessionID, DestinationSessionID: destinationSessionID,
			SourceBefore: currentDraft, DestinationBefore: destinationDraft,
			DestinationAfter: merged,
		}); err != nil {
			return authoring.Message{}, fmt.Errorf("transfer replacement session draft: %w", err)
		}
		return merged, nil
	case preserveSourceDraft:
		if destinationSessionID == s.sourceSessionID {
			return destinationDraft, nil
		}
		if currentDraft.Equal(s.baseline) {
			return destinationDraft, nil
		}
		merged, err := workbench.MergeSessionDraft(destinationDraft, currentDraft)
		if err != nil {
			return authoring.Message{}, fmt.Errorf("merge session draft: %w", err)
		}
		if err := store.ApplyDraftTransfer(workbench.DraftTransfer{
			SourceSessionID: s.sourceSessionID, DestinationSessionID: destinationSessionID,
			SourceBefore: currentDraft, SourceAfter: s.baseline,
			DestinationBefore: destinationDraft, DestinationAfter: merged,
		}); err != nil {
			return authoring.Message{}, fmt.Errorf("transfer session draft: %w", err)
		}
		return merged, nil
	default:
		return authoring.Message{}, errors.New("session draft transition has an invalid source disposition")
	}
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
	if activateSessionStateErr := l.workbench.ActivateSessionState(session.ID); activateSessionStateErr != nil {
		return authoring.Message{}, nil, fmt.Errorf("activate destination session state: %w", activateSessionStateErr)
	}
	draft, _ := l.workbench.Draft(session.ID)
	if rememberWorkspaceErr := l.workbench.RememberWorkspace(session.Workspace.Path); rememberWorkspaceErr != nil {
		return authoring.Message{}, nil, fmt.Errorf("remember workspace: %w", rememberWorkspaceErr)
	}
	transition := l.state.draftTransition
	if transition != nil {
		if _, transitionErr := transition.resolve(l.workbench, session.ID, draft, current); transitionErr != nil {
			return authoring.Message{}, nil, transitionErr
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
	if l.workbench != nil {
		if err := l.workbench.RetireSessionState(sessionID); err != nil {
			discarded := 0
			if l.queue != nil {
				before := len(l.queue.Snapshot(sessionID).Entries)
				err = errors.Join(err, l.queue.Restore(sessionID))
				discarded = before - len(l.queue.Snapshot(sessionID).Entries)
			}
			return discarded, fmt.Errorf("discard session authoring state: %w", err)
		}
	}
	discarded := 0
	if l.queue != nil {
		discarded = l.queue.ForgetSession(sessionID)
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
	l.prepareSessionProjectionReplacement(s.snapshot.Session, s.projection.conversation)
	l.root.cancelPluginCommands()
	l.tasks.operations.CancelScope(sessionOperationScope)
	l.runs.dropStream()
	l.root.dismissCompletion()
	l.setActiveSession(s.snapshot.Session)
	l.queue.ReleaseDispatch(previousSessionID)
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
		if l.state.invalidated {
			l.refreshInvalidatedSession(refreshAfterChange)
		}
	}
	l.steering.restoreSteerReceipts(s.snapshot)
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
