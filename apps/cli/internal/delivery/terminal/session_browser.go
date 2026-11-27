package terminal

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/delivery/format"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

const (
	renameSessionDialogHeight = 11
	deleteSessionDialogHeight = 8
)

type sessionBrowser struct {
	pane                  *sessionCenterPane
	dialog                *presentationDialog
	renameDialog          *presentationDialog
	deleteDialog          *presentationDialog
	catalogIntent         sessionCatalogIntent
	catalogRefreshPending bool

	runtime      Runtime
	workbench    *workbench.Store
	replayPolicy mutation.ReplayPolicy
	session      *sessionState
	tasks        taskRunner
	status       *statusView
	stack        *workspaceScene
	theme        kit.Theme
	glyphs       kit.Glyphs
	clipboard    program.Clipboard

	notify       func(string)
	observing    func() bool
	adoptCurrent func(conversation.Session)
}

type sessionBrowserConfig struct {
	browser  sessionBrowser
	switchTo func(id string)
}

func newSessionBrowser(cfg sessionBrowserConfig) *sessionBrowser {
	b := &cfg.browser
	b.pane = newSessionCenterPane(b.theme, b.glyphs, func(session conversation.Session) {
		b.dismissCatalog()
		cfg.switchTo(session.ID)
	})
	b.pane.loadMore = b.loadMoreSessions
	b.pane.toggleFavorite = b.toggleSessionFavorite
	b.pane.rename = b.openSessionRename
	b.pane.delete = b.openSessionDelete
	b.dialog = newPresentationDialog(b.stack, kit.DialogConfig{
		Behavior: base.DialogConfig{
			Title:   "Sessions · Center",
			Content: b.pane.Element(),
		},
		Theme:  b.theme,
		Glyphs: b.glyphs,
		Where:  layout.Placement{Width: wideDialogWidth, Height: tallDialogHeight},
	})
	b.pane.picker.cancel = b.dismissCatalog
	return b
}

func (b *sessionBrowser) dismiss() {
	b.dismissCatalog()
	if b.renameDialog != nil {
		b.renameDialog.Dismiss()
		b.renameDialog = nil
	}
	if b.deleteDialog != nil {
		b.deleteDialog.Dismiss()
		b.deleteDialog = nil
	}
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
	form := kit.FormConfig{
		Behavior: base.FormConfig{
			Fields: []base.Field{field},
		},
		Theme:  b.theme,
		Glyphs: b.glyphs,
		Hints:  []keymap.Action{base.Submit, base.Cancel},
	}
	form.Behavior.Keys = base.DefaultFormKeys()
	var dressed *kit.Form
	var dialog *presentationDialog
	form.Behavior.Submit = func(base.SubmissionID) (scene.ActionState, error) {
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
	dialog = newPresentationDialog(b.stack, kit.DialogConfig{
		Behavior: base.DialogConfig{
			Title:   "Rename session",
			Content: formNavigation(dressed.Element(), leave),
		},
		Theme:  b.theme,
		Glyphs: b.glyphs,
		Where:  layout.Placement{Width: compactDialogWidth, Height: renameSessionDialogHeight},
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
	choice.SetOptions([]base.Option[bool]{{Label: cancelLabel, Value: false}, {Label: "Delete permanently", Value: true}})
	form := kit.FormConfig{
		Behavior: base.FormConfig{
			Fields: []base.Field{choice},
		},
		Theme:  b.theme,
		Glyphs: b.glyphs,
		Hints:  []keymap.Action{base.Submit, base.Cancel},
	}
	form.Behavior.Keys = base.DefaultFormKeys()
	var dialog *presentationDialog
	form.Behavior.Submit = func(base.SubmissionID) (scene.ActionState, error) {
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
	dialog = newPresentationDialog(b.stack, kit.DialogConfig{
		Behavior: base.DialogConfig{
			Title:   "Delete session",
			Content: formNavigation(dressed.Element(), leave),
		},
		Theme:  b.theme,
		Glyphs: b.glyphs,
		Where:  layout.Placement{Width: compactDialogWidth, Height: deleteSessionDialogHeight},
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
			if updated.ID == b.session.current.ID {
				b.adoptCurrent(updated)
			}
			b.refreshOpenCatalog()
			b.notify(label + " complete")
			if settled != nil {
				settled(nil)
			}
		},
	)
	if !started {
		b.notify(sessionCenterOperation.busy().Error())
	}
	return started
}

func (b *sessionBrowser) deleteSessionFromCenter(id string) {
	started := b.tasks.application(sessionCenterOperation,
		func(ctx context.Context) (session.ConfirmedDeletion, error) {
			return session.Delete(
				ctx, b.runtime, b.workbench, id, b.replayPolicy, runtimeRecoveryBackoff,
			)
		},
		func(result session.ConfirmedDeletion, err error) {
			if err != nil {
				b.notify(err.Error())
				return
			}
			b.refreshOpenCatalog()
			if result.LocalCleanupError != nil {
				b.notify("session deleted. Local state cleanup is pending (" + result.LocalCleanupError.Error() + ")")
				return
			}
			b.notify("deleted session")
		},
	)
	if !started {
		b.notify(sessionCenterOperation.busy().Error())
	}
}

func compactRelativeAge(at time.Time) string {
	if at.IsZero() {
		return "never"
	}
	amount, unit, recent := format.Elapsed(time.Since(at))
	if recent {
		return "now"
	}
	return strconv.Itoa(amount) + unit
}
