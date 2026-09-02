package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/attachment"
	"github.com/Tangerg/flame/cli/internal/application/agent/mutation"
	"github.com/Tangerg/flame/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func (a *app) ShowSessions() {
	if a.execution.observing() {
		a.message("finish or cancel the current run before switching sessions")
		return
	}
	a.dialogs.sessionCenter.Reset()
	a.loadSessionPage("", false)
}

func (a *app) loadMoreSessions() {
	if !a.dialogs.sessionCenter.HasMore() {
		a.message("all sessions are already loaded")
		return
	}
	a.loadSessionPage(a.dialogs.sessionCenter.Cursor(), true)
}

func (a *app) loadSessionPage(cursor string, appendPage bool) {
	a.message("loading sessions")
	a.runOperation(pickerCatalogOperation, true,
		func(ctx context.Context) (agent.SessionPage, error) {
			page, err := a.runtime.ListSessions(ctx, agent.SessionQuery{PageSize: agent.DefaultPageSize(), Cursor: cursor})
			if err != nil {
				return agent.SessionPage{}, err
			}
			if err := page.Validate(); err != nil {
				return agent.SessionPage{}, fmt.Errorf("list sessions: %w", err)
			}
			return page, nil
		},
		func(page agent.SessionPage, err error) {
			if appendPage && !a.dialogs.sessionDialog.Open() {
				return
			}
			if err != nil {
				a.message("could not load sessions: " + err.Error())
				return
			}
			if err := a.dialogs.sessionCenter.SetPage(page, appendPage); err != nil {
				a.message("could not load sessions: " + err.Error())
				return
			}
			if !appendPage {
				a.dialogs.sessionDialog.Show()
			}
			a.status.note("choose a session")
		},
	)
}

func (a *app) toggleSessionFavorite(session agent.Session) {
	desired := !session.Favorite
	a.updateSessionFromCenter(session.ID, "updating favorite", func(latest agent.Session) agent.UpdateSession {
		return agent.UpdateSession{SessionID: latest.ID, Favorite: &desired, ExpectedRevision: latest.Revision}
	})
}

func (a *app) openSessionRename(session agent.Session) {
	title := displayTitle(session)
	field := &headless.Text{Label: "Session title", Value: headless.Bind(&title), Check: requiredText}
	field.Editor().Clipboard = a.loop.Clipboard()
	form := headless.NewForm(field)
	form.Keys = headless.DefaultFormKeys()
	var dialog *kit.Dialog
	form.Done = func() {
		if a.dialogs.sessionRenameDialog != dialog {
			return
		}
		dialog.Controller().Dismiss()
		a.dialogs.sessionRenameDialog = nil
		trimmed := strings.TrimSpace(title)
		a.updateSessionFromCenter(session.ID, "renaming session", func(latest agent.Session) agent.UpdateSession {
			return agent.UpdateSession{SessionID: latest.ID, Title: &trimmed, ExpectedRevision: latest.Revision}
		})
	}
	form.GaveUp = func() {
		if a.dialogs.sessionRenameDialog == dialog {
			dialog.Controller().Dismiss()
			a.dialogs.sessionRenameDialog = nil
		}
	}
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: "Rename session", Body: dressed,
		Where: layout.Placement{Width: 68, Height: 7},
	})
	a.dialogs.sessionRenameDialog = dialog
	dialog.Controller().Show()
}

func (a *app) openSessionDelete(session agent.Session) {
	if session.ID == a.session.current.ID {
		a.message("switch away before deleting the current session")
		return
	}
	confirmed := false
	choice := &headless.Select[bool]{Label: "Delete " + displayTitle(session) + "?", Value: headless.Bind(&confirmed), Rows: 2}
	choice.SetOptions([]headless.Option[bool]{{Label: "Cancel", Value: false}, {Label: "Delete permanently", Value: true}})
	form := headless.NewForm(choice)
	form.Keys = headless.DefaultFormKeys()
	var dialog *kit.Dialog
	form.Done = func() {
		if a.dialogs.sessionDeleteDialog != dialog {
			return
		}
		dialog.Controller().Dismiss()
		a.dialogs.sessionDeleteDialog = nil
		if confirmed {
			a.deleteSessionFromCenter(session.ID)
		}
	}
	form.GaveUp = func() {
		if a.dialogs.sessionDeleteDialog == dialog {
			dialog.Controller().Dismiss()
			a.dialogs.sessionDeleteDialog = nil
		}
	}
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: "Delete session", Body: dressed,
		Where: layout.Placement{Width: 68, Height: 8},
	})
	a.dialogs.sessionDeleteDialog = dialog
	dialog.Controller().Show()
}

func (a *app) updateSessionFromCenter(id, label string, build func(agent.Session) agent.UpdateSession) {
	started := a.runApplicationOperation(sessionCenterOperation, false,
		func(ctx context.Context) (agent.Session, error) {
			latest, err := a.runtime.GetSession(ctx, id)
			if err != nil {
				return agent.Session{}, err
			}
			return session.Update(ctx, a.runtime, build(latest.Session))
		},
		func(updated agent.Session, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			a.dialogs.sessionCenter.Upsert(updated)
			if updated.ID == a.session.current.ID {
				a.setActiveSession(updated)
			}
			a.message(label + " complete")
		},
	)
	if !started {
		a.message("wait for the current session action to finish")
	}
}

func (a *app) deleteSessionFromCenter(id string) {
	started := a.runApplicationOperation(sessionCenterOperation, false,
		func(ctx context.Context) (session.DeletionResult, error) {
			return session.Delete(
				ctx, a.runtime, a.workbench, id, commandReplayPolicy(a.runtimeProfile), runtimeRecoveryBackoff,
			)
		},
		func(result session.DeletionResult, err error) {
			switch result.Outcome {
			case mutation.Rejected:
				if rejectErr := session.RejectDeletion(a.workbench, result); rejectErr != nil {
					a.message("delete session failed; local intent cleanup failed: " + errors.Join(err, rejectErr).Error())
					return
				}
				a.message("delete session failed: " + err.Error())
				return
			case mutation.Unknown:
				if result.Request.CommandID == "" {
					a.message("delete session failed: " + err.Error())
					return
				}
				a.message("delete session outcome is unknown; it will be reconciled on restart: " + err.Error())
				return
			case mutation.Confirmed:
			default:
				a.message("delete session returned an invalid settlement outcome")
				return
			}
			if err := session.ConfirmDeletion(a.workbench, result); err != nil {
				a.message("deleted session; local state cleanup failed: " + err.Error())
				return
			}
			if a.queue != nil {
				a.queue.Clear(result.Request.SessionID)
			}
			a.dialogs.sessionCenter.Remove(id)
			a.message("deleted session")
		},
	)
	if !started {
		a.message("wait for the current session action to finish")
	}
}

func (a *app) NewSession() {
	a.startSessionInWorkspace(a.session.current.Workspace.Path)
}

func (a *app) RenameSession(title string) {
	if a.execution.observing() {
		a.message("finish or cancel the current run before renaming the session")
		return
	}
	title = strings.TrimSpace(title)
	if title == "" {
		a.message("/rename needs a non-empty title")
		return
	}
	sessionID := a.session.current.ID
	a.runSessionChange("renaming session",
		func(ctx context.Context) (agent.Session, error) {
			latest, err := a.runtime.GetSession(ctx, sessionID)
			if err != nil {
				return agent.Session{}, err
			}
			return session.Update(ctx, a.runtime, agent.UpdateSession{
				SessionID: sessionID, Title: &title, ExpectedRevision: latest.Session.Revision,
			})
		},
		func(updated agent.Session) error {
			a.setActiveSession(updated)
			a.message("renamed session to " + updated.Title)
			return nil
		},
	)
}

func (a *app) ForkSession(title string) {
	source := a.session.current.ID
	a.runSessionChange("forking session",
		func(ctx context.Context) (agent.SessionSnapshot, error) {
			forked, err := a.runtime.ForkSession(ctx, agent.ForkSession{SessionID: source, Title: strings.TrimSpace(title)})
			if err != nil {
				return agent.SessionSnapshot{}, err
			}
			return a.readSessionAfterMutation(ctx, forked.ID)
		},
		func(snapshot agent.SessionSnapshot) error { return a.installSnapshot(snapshot) },
	)
}

func (a *app) forkSessionFromRun(runID string) {
	source := a.session.current.ID
	short := shortIdentity(runID)
	a.runSessionChange("forking session from "+short,
		func(ctx context.Context) (agent.SessionSnapshot, error) {
			forked, err := a.runtime.ForkSession(ctx, agent.ForkSession{
				SessionID: source, FromRunID: runID, Title: "Fork from " + short,
			})
			if err != nil {
				return agent.SessionSnapshot{}, err
			}
			return a.readSessionAfterMutation(ctx, forked.ID)
		},
		func(snapshot agent.SessionSnapshot) error { return a.installSnapshot(snapshot) },
	)
}

func (a *app) switchSession(id string) {
	if id == a.session.current.ID {
		a.message("already in " + displayTitle(a.session.current))
		return
	}
	a.runSessionChange("loading session",
		func(ctx context.Context) (agent.SessionSnapshot, error) { return a.runtime.GetSession(ctx, id) },
		func(snapshot agent.SessionSnapshot) error { return a.installSnapshot(snapshot) },
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
	if a.execution.observing() {
		a.message("finish or cancel the current run before changing sessions")
		return
	}
	if a.execution.pendingCancel != nil {
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
	a.dialogs.sessionDialog.Dismiss()
	baseline, _, err := a.currentDraft()
	if err != nil {
		a.message(label + " failed: " + err.Error())
		return
	}
	if err := a.saveDraft(baseline); err != nil {
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
	if !a.runOperation(sessionChangeOperation, false, work, func(result T, err error) {
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
	if a.session.invalidated && a.execution.conversation.Phase() != agent.ConversationRunning &&
		!a.execution.following && a.execution.pendingCancel == nil {
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
	baseline        agent.Message
	disposition     sourceDraftDisposition
}

func (s sessionDraftTransition) resolve(
	store *workbench.Store,
	destinationSessionID string,
	destinationDraft agent.Message,
	currentDraft agent.Message,
) (agent.Message, error) {
	switch s.disposition {
	case retireSourceDraft:
		if destinationSessionID == s.sourceSessionID {
			return agent.Message{}, fmt.Errorf("replacement session reused retired identity %s", destinationSessionID)
		}
		merged, err := workbench.MergeSessionDraft(destinationDraft, currentDraft)
		if err != nil {
			return agent.Message{}, fmt.Errorf("merge replacement session draft: %w", err)
		}
		if strings.TrimSpace(currentDraft.Text) == "" && len(currentDraft.Attachments) == 0 {
			return merged, nil
		}
		if err := store.ApplyDraftTransfer(workbench.DraftTransfer{
			SourceSessionID: s.sourceSessionID, DestinationSessionID: destinationSessionID,
			SourceBefore: currentDraft, DestinationBefore: destinationDraft,
			DestinationAfter: merged,
		}); err != nil {
			return agent.Message{}, fmt.Errorf("transfer replacement session draft: %w", err)
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
			return agent.Message{}, fmt.Errorf("merge session draft: %w", err)
		}
		if err := store.ApplyDraftTransfer(workbench.DraftTransfer{
			SourceSessionID: s.sourceSessionID, DestinationSessionID: destinationSessionID,
			SourceBefore: currentDraft, SourceAfter: s.baseline,
			DestinationBefore: destinationDraft, DestinationAfter: merged,
		}); err != nil {
			return agent.Message{}, fmt.Errorf("transfer session draft: %w", err)
		}
		return merged, nil
	default:
		return agent.Message{}, errors.New("session draft transition has an invalid source disposition")
	}
}

type sessionInstallation struct {
	snapshot         agent.SessionSnapshot
	attachments      *attachment.Resolver
	projection       sessionProjection
	draft            agent.Message
	rollbackRecovery *workbench.SessionRollbackRecovery
}

func (a *app) prepareSessionInstallation(snapshot agent.SessionSnapshot) (sessionInstallation, error) {
	attachments, err := attachment.New(snapshot.Session.Workspace.Path)
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
	session agent.Session,
) (agent.Message, *workbench.SessionRollbackRecovery, error) {
	current, _, err := a.currentDraft()
	if err != nil {
		return agent.Message{}, nil, err
	}
	if saveDraftErr := a.saveDraft(current); saveDraftErr != nil {
		return agent.Message{}, nil, fmt.Errorf("save source session draft: %w", saveDraftErr)
	}
	if activateSessionStateErr := a.workbench.ActivateSessionState(session.ID); activateSessionStateErr != nil {
		return agent.Message{}, nil, fmt.Errorf("activate destination session state: %w", activateSessionStateErr)
	}
	draft, _ := a.workbench.Draft(session.ID)
	if rememberWorkspaceErr := a.workbench.RememberWorkspace(session.Workspace.Path); rememberWorkspaceErr != nil {
		return agent.Message{}, nil, fmt.Errorf("remember workspace: %w", rememberWorkspaceErr)
	}
	transition := a.session.draftTransition
	if transition != nil {
		if _, transitionErr := transition.resolve(a.workbench, session.ID, draft, current); transitionErr != nil {
			return agent.Message{}, nil, transitionErr
		}
	}
	// Draft transfer is the last separate preparation that can abort projection
	// installation. Activation then materializes rollback recovery and retires
	// its one-time report in the same durable Session state replacement.
	activation, err := a.workbench.ActivateSessionDraft(session.ID, agent.Message{})
	if err != nil {
		return agent.Message{}, nil, fmt.Errorf("activate destination session draft: %w", err)
	}
	return activation.Draft, activation.Rollback, nil
}

func (a *app) retireSessionState(sessionID string) (int, error) {
	if a.workbench != nil {
		if err := a.workbench.RetireSessionState(sessionID); err != nil {
			return 0, fmt.Errorf("discard session authoring state: %w", err)
		}
	}
	discarded := 0
	if a.queue != nil {
		discarded = a.queue.Clear(sessionID)
	}
	return discarded, nil
}

// readSessionAfterMutation converges the authoritative projection without
// repeating a mutation that may already be durable. Its retry budget is the
// same user-configured transport policy as live run recovery.
func (a *app) readSessionAfterMutation(ctx context.Context, sessionID string) (agent.SessionSnapshot, error) {
	policy := a.reconnectPolicy
	for failures := 0; ; {
		snapshot, err := a.runtime.GetSession(ctx, sessionID)
		if err == nil {
			return snapshot, nil
		}
		failures++
		delay, shouldRetry, policyErr := policy.Next(failures, err)
		if policyErr != nil {
			return agent.SessionSnapshot{}, policyErr
		}
		if !shouldRetry {
			return agent.SessionSnapshot{}, err
		}
		if err := retry.Wait(ctx, delay); err != nil {
			return agent.SessionSnapshot{}, err
		}
	}
}

func (s sessionInstallation) apply(a *app) {
	previousSessionID := a.session.current.ID
	previousWorkspace := a.session.current.Workspace
	a.prepareSessionProjectionReplacement(s.snapshot.Session, s.projection.conversation)
	a.cancelPluginCommands()
	a.operations.CancelScope(sessionOperationScope)
	a.dropStream()
	a.completion.Dismiss()
	previousTranscript := a.transcript
	a.setActiveSession(s.snapshot.Session)
	a.queue.ReleaseDispatch(previousSessionID)
	a.execution.openingRunID = ""
	a.execution.conversation = s.projection.conversation
	a.execution.projectionFailed = false
	a.attachments = s.attachments
	a.transcript = s.projection.transcript
	a.wireTranscript(s.projection.transcript)
	a.restoreComposer(s.draft)
	a.draftState.Reset(a.session.current.ID, s.draft)
	a.activity.Reset()
	a.status.Reset()
	a.workbenchHealth.enterSession()
	a.status.setProblem(a.workbenchHealth.problem())
	a.header.SetUsage(s.projection.conversation.Usage())
	a.prompt.SetOptions(a.displayOptions())
	a.prompt.SetBusy(s.projection.conversation.Busy())
	a.shell.SetTranscript(s.projection.transcript)
	a.syncQueue()
	previousTranscript.Close()
	a.listenForSearch()
	a.setWindowTitle()
	a.restoreActivity(s.snapshot)
	a.restoreSessionOutbox()
	if a.session.current.Workspace != previousWorkspace {
		a.followRuntimeChanges()
	}
	if a.execution.conversation.Phase() == agent.ConversationIdle {
		a.message("session · " + displayTitle(s.snapshot.Session))
		if a.session.invalidated {
			a.refreshInvalidatedSession(false)
		}
	}
}

func (a *app) installSnapshot(snapshot agent.SessionSnapshot) error {
	installation, err := a.prepareSessionInstallation(snapshot)
	if err != nil {
		return err
	}
	installation.apply(a)
	if installation.rollbackRecovery != nil {
		a.reportSessionRollbackRecovery(*installation.rollbackRecovery)
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
