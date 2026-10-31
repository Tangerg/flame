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
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
)

// sessionBrowser is the Session center: it lists, favorites, renames and
// deletes Sessions and hands the chosen one to switchTo. It changes the
// presented Session only through adoptCurrent.
type sessionBrowser struct {
	pane         *sessionCenterPane
	dialog       *presentationDialog
	renameDialog *kit.Dialog
	deleteDialog *kit.Dialog

	runtime      Runtime
	workbench    *workbench.Store
	queue        *workbench.Queue
	replayPolicy mutation.ReplayPolicy
	session      *sessionState
	tasks        taskRunner
	status       *statusView
	stack        *headless.Stack
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
	b.dialog = newPresentationDialog(kit.DialogConfig{
		Stack: b.stack, Theme: b.theme, Glyphs: b.glyphs, Title: "Sessions · Center", Body: b.pane,
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
		b.renameDialog.Controller().Dismiss()
		b.renameDialog = nil
	}
	if b.deleteDialog != nil {
		b.deleteDialog.Controller().Dismiss()
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
			if !appendPage {
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
	field := headless.NewText(headless.TextConfig{Label: "Session title", Value: headless.Bind(&title), Check: requiredText})
	field.Clipboard = b.clipboard
	form := headless.NewForm(field)
	form.Keys = headless.DefaultFormKeys()
	dressed := kit.NewForm(kit.FormConfig{
		Theme: b.theme, Glyphs: b.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	var dialog *kit.Dialog
	form.Done = func() {
		if b.renameDialog != dialog {
			return
		}
		trimmed := strings.TrimSpace(title)
		if trimmed == strings.TrimSpace(session.Title) {
			dialog.Controller().Dismiss()
			b.renameDialog = nil
			return
		}
		started := b.updateSessionFromCenter(conversation.UpdateSession{
			SessionID: session.ID, Title: &trimmed, ExpectedRevision: session.Revision,
		}, "renaming session", func(err error) {
			if b.renameDialog != dialog {
				return
			}
			if err != nil {
				dressed.Title = "Rename failed: " + err.Error()
				if errors.Is(err, conversation.ErrRevisionConflict) {
					dressed.Title += "\nClose this edit and reopen Sessions to load the current title."
				}
				dialog.Controller().Show()
				return
			}
			b.renameDialog = nil
		})
		if started {
			dialog.Controller().Dismiss()
		} else {
			dressed.Title = "Wait for the current Session action to finish."
		}
	}
	form.GaveUp = func() {
		if b.renameDialog == dialog {
			dialog.Controller().Dismiss()
			b.renameDialog = nil
		}
	}
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: b.stack, Theme: b.theme, Glyphs: b.glyphs,
		Title: "Rename session", Body: dressed,
		Where: layout.Placement{Width: 68, Height: 11},
	})
	b.renameDialog = dialog
	dialog.Controller().Show()
}

func (b *sessionBrowser) openSessionDelete(session conversation.Session) {
	if session.ID == b.session.current.ID {
		b.notify("switch away before deleting the current session")
		return
	}
	confirmed := false
	choice := headless.NewSelect(headless.SelectConfig[bool]{Same: headless.Equal[bool], Label: "Delete " + displayTitle(session) + "?", Value: headless.Bind(&confirmed), Rows: 2})
	choice.SetOptions([]headless.Option[bool]{{Label: "Cancel", Value: false}, {Label: "Delete permanently", Value: true}})
	form := headless.NewForm(choice)
	form.Keys = headless.DefaultFormKeys()
	var dialog *kit.Dialog
	form.Done = func() {
		if b.deleteDialog != dialog {
			return
		}
		dialog.Controller().Dismiss()
		b.deleteDialog = nil
		if confirmed {
			b.deleteSessionFromCenter(session.ID)
		}
	}
	form.GaveUp = func() {
		if b.deleteDialog == dialog {
			dialog.Controller().Dismiss()
			b.deleteDialog = nil
		}
	}
	dressed := kit.NewForm(kit.FormConfig{
		Theme: b.theme, Glyphs: b.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: b.stack, Theme: b.theme, Glyphs: b.glyphs,
		Title: "Delete session", Body: dressed,
		Where: layout.Placement{Width: 68, Height: 8},
	})
	b.deleteDialog = dialog
	dialog.Controller().Show()
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

func (a *app) NewSession() {
	a.startSessionInWorkspace(a.session.current.Workspace.Path)
}

func (a *app) RenameSession(title string) {
	if a.runs.observing() {
		a.message("finish or cancel the current run before renaming the session")
		return
	}
	title = strings.TrimSpace(title)
	if title == "" {
		a.message("/rename needs a non-empty title")
		return
	}
	target := a.session.current
	a.runSessionChange("renaming session",
		func(ctx context.Context) (conversation.Session, error) {
			return a.runtime.UpdateSession(ctx, conversation.UpdateSession{
				SessionID: target.ID, Title: &title, ExpectedRevision: target.Revision,
			})
		},
		func(updated conversation.Session) error {
			a.setActiveSession(updated)
			a.message("renamed session to " + updated.Title)
			return nil
		},
	)
}

func (a *app) ForkSession(title string) {
	source := a.session.current.ID
	a.runSessionChange("forking session",
		func(ctx context.Context) (conversation.SessionSnapshot, error) {
			forked, err := a.runtime.ForkSession(ctx, conversation.ForkSession{SessionID: source, Title: strings.TrimSpace(title)})
			if err != nil {
				return conversation.SessionSnapshot{}, err
			}
			return a.readSessionAfterMutation(ctx, forked.ID)
		},
		func(snapshot conversation.SessionSnapshot) error { return a.installSnapshot(snapshot) },
	)
}

func (a *app) forkSessionFromRun(runID string) {
	source := a.session.current.ID
	short := shortIdentity(runID)
	a.runSessionChange("forking session from "+short,
		func(ctx context.Context) (conversation.SessionSnapshot, error) {
			forked, err := a.runtime.ForkSession(ctx, conversation.ForkSession{
				SessionID: source, FromRunID: runID, Title: "Fork from " + short,
			})
			if err != nil {
				return conversation.SessionSnapshot{}, err
			}
			return a.readSessionAfterMutation(ctx, forked.ID)
		},
		func(snapshot conversation.SessionSnapshot) error { return a.installSnapshot(snapshot) },
	)
}

func (a *app) switchSession(id string) {
	if id == a.session.current.ID {
		a.message("already in " + displayTitle(a.session.current))
		return
	}
	a.runSessionChange("loading session",
		func(ctx context.Context) (conversation.SessionSnapshot, error) { return a.runtime.GetSession(ctx, id) },
		func(snapshot conversation.SessionSnapshot) error { return a.installSnapshot(snapshot) },
	)
}

func (a *app) runSessionChange[T any](label string, work func(context.Context) (T, error), apply func(T) error) {
	a.runSessionChangeWithDraftDisposition(label, preserveSourceDraft, work, apply)
}

func (a *app) runSessionChangeWithDraftDisposition[T any](
	label string,
	disposition sourceDraftDisposition,
	work func(context.Context) (T, error),
	apply func(T) error,
) {
	if a.runs.observing() {
		a.message("finish or cancel the current run before changing sessions")
		return
	}
	if a.runs.cancelPending() {
		a.message("wait for runtime cancellation to finish")
		return
	}
	if a.operations.Active(sessionChangeOperation) {
		a.message("wait for the current session change to finish")
		return
	}
	if a.operations.Active(sessionCenterOperation) {
		a.message("wait for the current session action to finish")
		return
	}
	a.operations.Cancel(pickerCatalogOperation)
	a.sessions.dialog.Dismiss()
	baseline, _, err := a.composer.draft()
	if err != nil {
		a.message(label + " failed: " + err.Error())
		return
	}
	if err := a.composer.saveDraft(baseline); err != nil {
		a.reportWorkbenchIssue(workbenchDraft, err)
		a.message("session change blocked: save session draft: " + err.Error())
		return
	}
	a.reportWorkbenchIssue(workbenchDraft, nil)
	a.session.draftTransition = &sessionDraftTransition{
		sourceSessionID: a.session.current.ID,
		baseline:        baseline,
		disposition:     disposition,
	}
	a.message(label)
	if !a.tasks.session(sessionChangeOperation, false, work, func(result T, err error) {
		defer a.settleSessionChange()
		if err != nil {
			a.message(label + " failed: " + err.Error())
			return
		}
		if err := apply(result); err != nil {
			a.message(label + " failed: " + err.Error())
		}
	}) {
		a.settleSessionChange()
		a.message("wait for the current session change to finish")
	}
}

// cancelSessionChange abandons the pending projection replacement while
// retaining the composer state authored during it. Runtime mutations that
// completed before cancellation remain discoverable through the session list;
// the terminal only withdraws its unfinished local transition.
func (a *app) cancelSessionChange() bool {
	if !a.operations.Active(sessionChangeOperation) {
		return false
	}
	a.operations.Cancel(sessionChangeOperation)
	a.message("session change canceled")
	a.settleSessionChange()
	return true
}

// settleSessionChange closes the terminal-side draft transaction and resumes
// any authoritative refresh that runtime notifications deferred behind it.
func (a *app) settleSessionChange() {
	a.session.draftTransition = nil
	if a.session.invalidated && a.runs.conversation().Phase() != conversation.Running &&
		!a.runs.following() && !a.runs.cancelPending() {
		a.refreshInvalidatedSession(false)
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

func (a *app) prepareSessionInstallation(snapshot conversation.SessionSnapshot) (sessionInstallation, error) {
	attachments, err := attachment.New(authoringDirectory(a.localDirectory, snapshot.Session.Workspace.Path))
	if err != nil {
		return sessionInstallation{}, fmt.Errorf("session attachments: %w", err)
	}
	projection, err := a.projectSession(snapshot, nil)
	if err != nil {
		return sessionInstallation{}, fmt.Errorf("install session: %w", err)
	}
	draft, recovery, err := a.prepareDestinationDraft(snapshot.Session)
	if err != nil {
		projection.close()
		return sessionInstallation{}, err
	}
	return sessionInstallation{
		snapshot: snapshot, attachments: attachments, projection: projection, draft: draft,
		rollbackRecovery: recovery,
	}, nil
}

func (a *app) prepareDestinationDraft(
	session conversation.Session,
) (authoring.Message, *workbench.SessionRollbackRecovery, error) {
	current, _, err := a.composer.draft()
	if err != nil {
		return authoring.Message{}, nil, err
	}
	if saveDraftErr := a.composer.saveDraft(current); saveDraftErr != nil {
		return authoring.Message{}, nil, fmt.Errorf("save source session draft: %w", saveDraftErr)
	}
	if activateSessionStateErr := a.workbench.ActivateSessionState(session.ID); activateSessionStateErr != nil {
		return authoring.Message{}, nil, fmt.Errorf("activate destination session state: %w", activateSessionStateErr)
	}
	draft, _ := a.workbench.Draft(session.ID)
	if rememberWorkspaceErr := a.workbench.RememberWorkspace(session.Workspace.Path); rememberWorkspaceErr != nil {
		return authoring.Message{}, nil, fmt.Errorf("remember workspace: %w", rememberWorkspaceErr)
	}
	transition := a.session.draftTransition
	if transition != nil {
		if _, transitionErr := transition.resolve(a.workbench, session.ID, draft, current); transitionErr != nil {
			return authoring.Message{}, nil, transitionErr
		}
	}
	// Draft transfer is the last separate preparation that can abort projection
	// installation. Activation then materializes rollback recovery and retires
	// its one-time report in the same durable Session state replacement.
	activation, err := a.workbench.ActivateSessionDraft(session.ID, authoring.Message{})
	if err != nil {
		return authoring.Message{}, nil, fmt.Errorf("activate destination session draft: %w", err)
	}
	return activation.Draft, activation.Rollback, nil
}

func (a *app) retireSessionState(sessionID string) (int, error) {
	if a.workbench != nil {
		if err := a.workbench.RetireSessionState(sessionID); err != nil {
			discarded := 0
			if a.queue != nil {
				before := len(a.queue.Snapshot(sessionID).Entries)
				err = errors.Join(err, a.queue.Restore(sessionID))
				discarded = before - len(a.queue.Snapshot(sessionID).Entries)
			}
			return discarded, fmt.Errorf("discard session authoring state: %w", err)
		}
	}
	discarded := 0
	if a.queue != nil {
		discarded = a.queue.ForgetSession(sessionID)
	}
	return discarded, nil
}

// readSessionAfterMutation converges the authoritative projection without
// repeating a mutation that may already be durable. Its caller owns cancellation,
// just as the observer owns live Run recovery.
func (a *app) readSessionAfterMutation(ctx context.Context, sessionID string) (conversation.SessionSnapshot, error) {
	for failures := 0; ; {
		snapshot, err := a.runtime.GetSession(ctx, sessionID)
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

func (s sessionInstallation) apply(a *app) {
	previousSessionID := a.session.current.ID
	previousWorkspace := a.session.current.Workspace
	a.prepareSessionProjectionReplacement(s.snapshot.Session, s.projection.conversation)
	a.cancelPluginCommands()
	a.operations.CancelScope(sessionOperationScope)
	a.runs.dropStream()
	a.completion.Dismiss()
	previousTranscript := a.transcript
	a.setActiveSession(s.snapshot.Session)
	a.queue.ReleaseDispatch(previousSessionID)
	a.runs.switchProjection(s.projection.conversation)
	a.composer.attachments = s.attachments
	a.transcript = s.projection.transcript
	a.wireTranscript(s.projection.transcript)
	a.composer.restoreComposer(s.draft)
	a.composer.drafts.adopt(a.session.current.ID, s.draft)
	a.activity.Reset()
	a.status.Reset()
	a.workbenchHealth.enterSession()
	a.status.setProblem(a.workbenchHealth.problem())
	a.header.SetUsage(s.projection.conversation.Usage())
	a.prompt.SetOptions(a.displayOptions())
	a.prompt.SetBusy(s.projection.conversation.Busy())
	a.shell.SetTranscript(s.projection.transcript)
	a.runs.syncQueue()
	previousTranscript.Close()
	a.listenForSearch()
	a.setWindowTitle()
	a.restoreActivity(s.snapshot)
	a.runs.restoreSessionOutbox()
	if a.session.current.Workspace != previousWorkspace {
		a.followRuntimeChanges()
	}
	if a.runs.conversation().Phase() == conversation.Idle {
		a.message("session · " + displayTitle(s.snapshot.Session))
		if a.session.invalidated {
			a.refreshInvalidatedSession(false)
		}
	}
	a.steering.restoreSteerReceipts(s.snapshot)
}

func (a *app) installSnapshot(snapshot conversation.SessionSnapshot) error {
	installation, err := a.prepareSessionInstallation(snapshot)
	if err != nil {
		return err
	}
	installation.apply(a)
	if installation.rollbackRecovery != nil {
		a.message(rollbackRecoveryNotice(*installation.rollbackRecovery))
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
