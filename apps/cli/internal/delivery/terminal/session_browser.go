package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
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
		Where: layout.Placement{Width: wideDialogWidth, Height: 24},
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
	b.loadSessionPage(firstSessionPage)
}

func (b *sessionBrowser) loadMoreSessions() {
	if !b.pane.HasMore() {
		b.notify("all sessions are already loaded")
		return
	}
	b.loadSessionPage(nextSessionPage)
}

func (b *sessionBrowser) loadSessionPage(position sessionPagePosition) {
	cursor := b.pane.requestCursor(position)
	b.notify("loading sessions")
	b.tasks.session(pickerCatalogOperation,
		func(ctx context.Context) (conversation.SessionPage, error) {
			return b.runtime.ListSessions(ctx, conversation.SessionQuery{PageSize: conversation.DefaultPageSize(), Cursor: cursor})
		},
		func(page conversation.SessionPage, err error) {
			if position == nextSessionPage && !b.dialog.Open() {
				return
			}
			if err != nil {
				b.notify("could not load sessions: " + err.Error())
				return
			}
			if err := b.pane.SetPage(page, position); err != nil {
				b.notify("could not load sessions: " + err.Error())
				return
			}
			if position == firstSessionPage && !b.dialog.Open() {
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
		Where: layout.Placement{Width: compactDialogWidth, Height: 11},
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
		Where: layout.Placement{Width: compactDialogWidth, Height: 8},
	})
	b.deleteDialog = dialog
	dialog.Show()
}

func (b *sessionBrowser) updateSessionFromCenter(request conversation.UpdateSession, label string, settled func(error)) bool {
	started := b.tasks.application(sessionCenterOperation,
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
	started := b.tasks.application(sessionCenterOperation,
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
