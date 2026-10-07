package terminal

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

const (
	saveContextDocument   keymap.Action = "save document"
	cancelContextDocument keymap.Action = "cancel editing"
)

// contextEditor is a reusable, multiline editor for durable context. Its value
// lives in the editor until Save succeeds, so validation or backend failures
// never destroy the user's draft.
type contextEditor struct {
	composer kit.Composer
	theme    kit.Theme
	keys     *keymap.Map
	save     func(string)
	cancel   func()
	phase    contextSavePhase
	failure  string
}

type contextSavePhase uint8

const (
	contextUnsaved contextSavePhase = iota
	contextSaving
	contextSaveFailed
	contextSavedWithNewEdits
)

func (c *contextEditor) notice() string {
	switch c.phase {
	case contextSaving:
		return "Saving…"
	case contextSaveFailed:
		return c.failure
	case contextSavedWithNewEdits:
		return "Saved. New edits remain unsaved."
	default:
		return ""
	}
}

type contextEditorRequest struct {
	Title       string
	Description string
	Content     string
	Placeholder string
	Save        func(string, func(error) bool) error
	Dismissed   func()
}

type contextEditorSession struct {
	dialog    *kit.Dialog
	editor    *contextEditor
	dismissed func()
	closed    bool
}

func (c *contextEditorSession) Dismiss() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	if c.dialog != nil {
		c.dialog.Controller().Dismiss()
	}
	if c.dismissed != nil {
		c.dismissed()
	}
}

func newContextEditor(theme kit.Theme, clipboard headless.Clipboard, content, placeholder string) *contextEditor {
	keys := headless.DefaultEditorKeys()
	keys.Bind(headless.InsertNewline, input.Chord{Code: input.Enter})
	keys.Bind(saveContextDocument, input.Ctrl.Rune('s'))
	keys.Bind(cancelContextDocument, input.Chord{Code: input.Esc})
	editor := &contextEditor{
		theme: theme, keys: keys,
		composer: kit.Composer{
			Theme: theme, MaxRows: 1_000,
		},
	}
	editor.composer.Editor().Keys = keys
	editor.composer.Editor().Clipboard = clipboard
	editor.composer.Editor().Placeholder = placeholder
	editor.composer.Editor().SetText(content)
	return editor
}

func (c *contextEditor) Draw(frame headless.Frame) {
	_, height := frame.Size()
	notice := c.notice()
	if notice == "" || height < 2 {
		c.composer.Draw(frame)
		return
	}
	rows := frame.Subs((layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Flex(1)}, {Size: layout.Fixed(1)},
	}))
	c.composer.Draw(rows[0])
	style := c.theme.Subtle
	if c.phase == contextSaveFailed {
		style = c.theme.Danger
	}
	rows[1].Text(0, 0, notice, style)
}

func (c *contextEditor) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() {
		action, _ := c.keys.Action(key.Chord())
		switch action {
		case saveContextDocument:
			if c.save != nil {
				c.save(c.composer.Editor().Text())
			}
			return true
		case cancelContextDocument:
			if c.cancel != nil {
				c.cancel()
			}
			return true
		}
	}
	return c.composer.Handle(event)
}

func (c *contextEditor) Focus(has bool) { c.composer.Focus(has) }

func (a *app) openContextEditor(request contextEditorRequest) *contextEditorSession {
	if a.dialogs.activeContextEditor != nil {
		a.dialogs.activeContextEditor.Dismiss()
	}
	editor := newContextEditor(a.transcript.theme, a.loop.Clipboard(), request.Content, request.Placeholder)
	var dialog *kit.Dialog
	session := &contextEditorSession{editor: editor}
	session.dismissed = func() {
		if a.dialogs.activeContextEditor == session {
			a.dialogs.activeContextEditor = nil
		}
		if request.Dismissed != nil {
			request.Dismissed()
		}
	}
	editor.cancel = session.Dismiss
	editor.save = func(value string) {
		if editor.phase == contextSaving || session.closed || a.dialogs.activeContextEditor != session {
			return
		}
		enter := func(phase contextSavePhase, failure string) {
			editor.phase, editor.failure = phase, failure
			if dialog != nil {
				dialog.Controller().SetDescription(editor.notice())
			}
		}
		enter(contextSaving, "")
		complete := func(err error) bool {
			if session.closed || a.dialogs.activeContextEditor != session {
				return false
			}
			if err != nil {
				enter(contextSaveFailed, err.Error())
				return false
			}
			if editor.composer.Editor().Text() != value {
				enter(contextSavedWithNewEdits, "")
				return false
			}
			session.Dismiss()
			return true
		}
		if err := request.Save(value, complete); err != nil {
			complete(err)
			a.message(request.Title + ": " + err.Error())
		}
	}
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: request.Title, Description: request.Description, Body: editor,
		Where: layout.Placement{Width: 100, Height: 24}, Keys: editor.keys,
		Hints: []keymap.Action{saveContextDocument, cancelContextDocument},
	})
	session.dialog = dialog
	a.dialogs.activeContextEditor = session
	dialog.Controller().Show()
	return session
}

func (a *app) dismissContextEditor() {
	if a.dialogs.activeContextEditor != nil {
		a.dialogs.activeContextEditor.Dismiss()
	}
}

var _ headless.Widget = (*contextEditor)(nil)
var _ headless.Interactive = (*contextEditor)(nil)
var _ headless.Focusable = (*contextEditor)(nil)
