package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/filesystem/attachment"
	"github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/retry"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

// sessionBrowser is the Session center: it lists, favorites, renames and
// deletes Sessions and hands the chosen one to switchTo. It changes the
// presented Session only through adoptCurrent.
type sessionBrowser struct {
	pane         *sessionCenterPane
	dialog       *presentationDialog
	renameDialog *presentationDialog
	deleteDialog *presentationDialog

	runtime      Runtime
	workbench    *workbench.Store
	queue        *workbench.Queue
	replayPolicy mutation.ReplayPolicy
	session      *sessionState
	tasks        taskRunner
	status       *statusView
	stack        *workspaceScene
	theme        kit.Theme
	glyphs       kit.Glyphs
	clipboard    program.Clipboard

	notify         func(string)
	observing      func() bool
	adoptCurrent   func(conversation.Session)
	catalogChanged func()
}

type sessionBrowserConfig struct {
	browser  sessionBrowser
	switchTo func(id string)
}

func newSessionBrowser(cfg sessionBrowserConfig) *sessionBrowser {
	b := &cfg.browser
	b.pane = newSessionCenterPane(b.theme, b.glyphs, func(session conversation.Session) {
		b.dialog.Dismiss()
		cfg.switchTo(session.ID)
	})
	b.pane.loadMore = b.loadMoreSessions
	b.pane.toggleFavorite = b.toggleSessionFavorite
	b.pane.rename = b.openSessionRename
	b.pane.delete = b.openSessionDelete
	b.dialog = newPresentationDialog(b.stack, kit.DialogConfig{Theme: b.theme, Glyphs: b.glyphs, Title: "Sessions · Center", Body: b.pane.Element(),
		Where: layout.Placement{Width: 96, Height: 24},
	})
	b.pane.picker.cancel = func() {
		b.tasks.operations.Cancel(pickerCatalogOperation)
		b.dialog.Dismiss()
	}
	return b
}

// dismiss closes the center and any rename or delete it opened.
func (b *sessionBrowser) dismiss() {
	b.dialog.Dismiss()
	if b.renameDialog != nil {
		b.renameDialog.Dismiss()
		b.renameDialog = nil
	}
	if b.deleteDialog != nil {
		b.deleteDialog.Dismiss()
		b.deleteDialog = nil
	}
}

func (b *sessionBrowser) ShowSessions() {
	if b.observing() {
		b.notify("finish or cancel the current run before switching sessions")
		return
	}
	b.pane.Reset()
	b.loadSessionPage("", false)
}

func (b *sessionBrowser) loadMoreSessions() {
	if !b.pane.HasMore() {
		b.notify("all sessions are already loaded")
		return
	}
	b.loadSessionPage(b.pane.Cursor(), true)
}

func (b *sessionBrowser) loadSessionPage(cursor string, appendPage bool) {
	b.notify("loading sessions")
	b.tasks.session(pickerCatalogOperation, true,
		func(ctx context.Context) (conversation.SessionPage, error) {
			return b.runtime.ListSessions(ctx, conversation.SessionQuery{PageSize: conversation.DefaultPageSize(), Cursor: cursor})
		},
		func(page conversation.SessionPage, err error) {
			if appendPage && !b.dialog.Open() {
				return
			}
			if err != nil {
				b.notify("could not load sessions: " + err.Error())
				return
			}
			if err := b.pane.SetPage(page, appendPage); err != nil {
				b.notify("could not load sessions: " + err.Error())
				return
			}
			if !appendPage && !b.dialog.Open() {
				b.dialog.Show()
			}
			b.status.note("choose a session")
		},
	)
}

func (b *sessionBrowser) toggleSessionFavorite(session conversation.Session) {
	desired := !session.Favorite
	b.updateSessionFromCenter(conversation.UpdateSession{
		SessionID: session.ID, Favorite: &desired, ExpectedRevision: session.Revision,
	}, "updating favorite", nil)
}

func (b *sessionBrowser) openSessionRename(session conversation.Session) {
	title := session.Title
	field := base.NewText(base.TextConfig{Label: "Session title", Initial: title, Check: requiredText})
	field.SetClipboard(b.clipboard)
	form := kit.FormConfig{Fields: []base.Field{field}, Theme: b.theme, Glyphs: b.glyphs,
		Hints: []keymap.Action{base.Submit, base.Cancel}}
	form.Keys = base.DefaultFormKeys()
	var dressed *kit.Form
	var dialog *presentationDialog
	form.Submit = func(base.SubmissionID) (scene.ActionState, error) {
		if b.renameDialog != dialog {
			return scene.Unchanged, nil
		}
		trimmed := strings.TrimSpace(field.Text())
		if trimmed == strings.TrimSpace(session.Title) {
			dialog.Dismiss()
			b.renameDialog = nil
			return scene.Unchanged, nil
		}
		started := b.updateSessionFromCenter(conversation.UpdateSession{
			SessionID: session.ID, Title: &trimmed, ExpectedRevision: session.Revision,
		}, "renaming session", func(err error) {
			if b.renameDialog != dialog {
				return
			}
			if err != nil {
				dressed.Controller().SetTitle("Rename failed: " + err.Error())
				if errors.Is(err, conversation.ErrRevisionConflict) {
					dressed.Controller().SetTitle(dressed.Controller().Title() + "\nClose this edit and reopen Sessions to load the current title.")
				}
				dialog.Show()
				return
			}
			b.renameDialog = nil
		})
		if started {
			dialog.Dismiss()
		} else {
			dressed.Controller().SetTitle("Wait for the current Session action to finish.")
		}
		return scene.Applied, nil
	}
	leave := func() {
		if b.renameDialog == dialog {
			dialog.Dismiss()
			b.renameDialog = nil
		}
	}
	dressed = kit.NewForm(form)
	dialog = newPresentationDialog(b.stack, kit.DialogConfig{Theme: b.theme, Glyphs: b.glyphs,
		Title: "Rename session", Body: formNavigation(dressed.Element(), leave),
		Where: layout.Placement{Width: 68, Height: 11},
	})
	b.renameDialog = dialog
	dialog.Show()
}

func (b *sessionBrowser) openSessionDelete(session conversation.Session) {
	if session.ID == b.session.current.ID {
		b.notify("switch away before deleting the current session")
		return
	}
	confirmed := false
	choice := base.NewSelect(base.SelectConfig[bool]{Same: base.Equal[bool], Label: "Delete " + displayTitle(session) + "?", Initial: confirmed, Rows: 2})
	choice.SetOptions([]base.Option[bool]{{Label: "Cancel", Value: false}, {Label: "Delete permanently", Value: true}})
	form := kit.FormConfig{Fields: []base.Field{choice}, Theme: b.theme, Glyphs: b.glyphs,
		Hints: []keymap.Action{base.Submit, base.Cancel}}
	form.Keys = base.DefaultFormKeys()
	var dialog *presentationDialog
	form.Submit = func(base.SubmissionID) (scene.ActionState, error) {
		if b.deleteDialog != dialog {
			return scene.Unchanged, nil
		}
		dialog.Dismiss()
		b.deleteDialog = nil
		if selected, ok := choice.Chosen(); ok && selected.Value {
			b.deleteSessionFromCenter(session.ID)
		}
		return scene.Applied, nil
	}
	leave := func() {
		if b.deleteDialog == dialog {
			dialog.Dismiss()
			b.deleteDialog = nil
		}
	}
	dressed := kit.NewForm(form)
	dialog = newPresentationDialog(b.stack, kit.DialogConfig{Theme: b.theme, Glyphs: b.glyphs,
		Title: "Delete session", Body: formNavigation(dressed.Element(), leave),
		Where: layout.Placement{Width: 68, Height: 8},
	})
	b.deleteDialog = dialog
	dialog.Show()
}

func (b *sessionBrowser) updateSessionFromCenter(request conversation.UpdateSession, label string, settled func(error)) bool {
	started := b.tasks.application(sessionCenterOperation, false,
		func(ctx context.Context) (conversation.Session, error) {
			return b.runtime.UpdateSession(ctx, request)
		},
		func(updated conversation.Session, err error) {
			if err != nil {
				if settled != nil {
					settled(err)
				}
				b.notify(label + " failed: " + err.Error())
				return
			}
			b.pane.Replace(updated)
			if updated.ID == b.session.current.ID {
				b.adoptCurrent(updated)
			}
			b.catalogChanged()
			b.notify(label + " complete")
			if settled != nil {
				settled(nil)
			}
		},
	)
	if !started {
		b.notify("wait for the current session action to finish")
	}
	return started
}

func (b *sessionBrowser) deleteSessionFromCenter(id string) {
	started := b.tasks.application(sessionCenterOperation, false,
		func(ctx context.Context) (session.DeletionResult, error) {
			return session.Delete(
				ctx, b.runtime, b.workbench, id, b.replayPolicy, runtimeRecoveryBackoff,
			)
		},
		func(result session.DeletionResult, err error) {
			switch result.Outcome {
			case mutation.Rejected:
				if rejectErr := session.RejectDeletion(b.workbench, result); rejectErr != nil {
					b.notify("delete session failed; local intent cleanup failed: " + errors.Join(err, rejectErr).Error())
					return
				}
				b.notify("delete session failed: " + err.Error())
				return
			case mutation.Unknown:
				if result.Request.CommandID == "" {
					b.notify("delete session failed: " + err.Error())
					return
				}
				b.notify("delete session outcome is unknown; the original request is saved for recovery; inspect the session before issuing a new command: " + err.Error())
				return
			case mutation.Confirmed:
			default:
				b.notify("delete session returned an invalid settlement outcome")
				return
			}
			if err := session.ConfirmDeletion(b.workbench, result); err != nil {
				if b.queue != nil {
					err = errors.Join(err, b.queue.Restore(result.Request.SessionID))
				}
				b.notify("deleted session; local state cleanup failed: " + err.Error())
				return
			}
			if b.queue != nil {
				b.queue.ForgetSession(result.Request.SessionID)
			}
			b.pane.Remove(id)
			b.notify("deleted session")
		},
	)
	if !started {
		b.notify("wait for the current session action to finish")
	}
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
		l.root.message("wait for the current session change to finish")
		return
	}
	if l.tasks.operations.Active(sessionCenterOperation) {
		l.root.message("wait for the current session action to finish")
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
	if !l.tasks.session(sessionChangeOperation, false, work, func(result T, err error) {
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
		l.root.message("wait for the current session change to finish")
	}
}

// cancelSessionChange abandons the pending projection replacement while
// retaining the composer state authored during it. Runtime mutations that
// completed before cancellation remain discoverable through the session list;
// the terminal only withdraws its unfinished local transition.
func (l *sessionLifecycle) cancelSessionChange() bool {
	if !l.tasks.operations.Active(sessionChangeOperation) {
		return false
	}
	l.tasks.operations.Cancel(sessionChangeOperation)
	l.root.message("session change canceled")
	l.settleSessionChange()
	return true
}

// settleSessionChange closes the terminal-side draft transaction and resumes
// any authoritative refresh that runtime notifications deferred behind it.
func (l *sessionLifecycle) settleSessionChange() {
	l.state.draftTransition = nil
	if l.state.invalidated && l.runs.conversation().Phase() != conversation.Running &&
		!l.runs.following() && !l.runs.cancelPending() {
		l.refreshInvalidatedSession(false)
	}
}

type sourceDraftDisposition uint8

const (
	preserveSourceDraft sourceDraftDisposition = iota
	retireSourceDraft
)

// sessionDraftTransition owns the authoring-state boundary while a session
// change is in flight. User-requested navigation preserves the source draft;
// forced replacement transfers it because the source session no longer exists.
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
		// Mutations such as rollback replace the authoritative projection without
		// changing session identity, so no cross-Session transfer is necessary.
		// Confirmed rollback input is materialized after this transition resolves.
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
	// Draft transfer is the last separate preparation that can abort projection
	// installation. Activation then materializes rollback recovery and retires
	// its one-time report in the same durable Session state replacement.
	activation, err := l.workbench.ActivateSessionDraft(session.ID, authoring.Message{})
	if err != nil {
		return authoring.Message{}, nil, fmt.Errorf("activate destination session draft: %w", err)
	}
	return activation.Draft, activation.Rollback, nil
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

// readSessionAfterMutation converges the authoritative projection without
// repeating a mutation that may already be durable. Its caller owns cancellation,
// just as the observer owns live Run recovery.
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

// install presents an installation as the current Session.
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
			l.refreshInvalidatedSession(false)
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

func compactRelativeAge(at time.Time) string {
	if at.IsZero() {
		return "never"
	}
	duration := time.Since(at)
	switch {
	case duration < time.Minute:
		return "now"
	case duration < time.Hour:
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%dh", int(duration.Hours()))
	default:
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	}
}
