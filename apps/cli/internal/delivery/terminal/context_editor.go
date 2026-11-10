package terminal

import (
	"image"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

const (
	saveContextDocument   keymap.Action = "save document"
	cancelContextDocument keymap.Action = "cancel editing"
)

// contextEditor is a reusable, multiline editor for durable context. Its value
// lives in the editor until Save succeeds, so validation or backend failures
// never destroy the user's draft.
type contextEditor struct {
	group    *scene.Group
	composer *kit.Composer
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
	dialog    *presentationDialog
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
		c.dialog.Dismiss()
	}
	if c.dismissed != nil {
		c.dismissed()
	}
}

func newContextEditor(theme kit.Theme, clipboard base.Clipboard, content, placeholder string) *contextEditor {
	keys := base.DefaultEditorKeys()
	keys.Bind(base.InsertNewline, input.Chord{Code: input.Enter})
	keys.Bind(saveContextDocument, input.Ctrl.Rune('s'))
	keys.Bind(cancelContextDocument, input.Chord{Code: input.Esc})
	editor := &contextEditor{
		theme: theme, keys: keys,
		composer: kit.NewComposer(kit.ComposerConfig{
			Theme: theme, MaxRows: 1_000,
		}),
	}
	editor.composer.Controller().SetKeys(keys)
	editor.composer.Controller().SetClipboard(clipboard)
	editor.composer.Controller().SetPlaceholder(placeholder)
	editor.composer.Controller().SetText(content)
	return editor
}

func (c *contextEditor) Element() scene.Element {
	if c.group == nil {
		c.group = scene.NewGroup(scene.GroupConfig{
			Items: []scene.Item{{Element: c.composer.Element(), Size: layout.Flex(1)}},
			Element: scene.ElementConfig{
				Preview: func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: c.Handle(event.Input())} },
				Paint: func(frame scene.Frame) {
					width, height := frame.Size()
					if height >= 2 {
						notice := c.notice()
						if notice == "" {
							notice = "ctrl+s save · esc close"
						}
						style := c.theme.Subtle
						if c.phase == contextSaveFailed {
							style = c.theme.Danger
						}
						frame.Sub(grid.Area(0, height-1, width, 1)).Text(0, 0, notice, style)
					}
				},
			},
			Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
				height := space.Y
				if height >= 2 {
					height--
				}
				return []image.Rectangle{grid.Area(0, 0, space.X, height)}
			},
		})
	}
	return c.group.Element()
}

func (c *contextEditor) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() {
		action, _ := c.keys.Action(key.Chord())
		switch action {
		case saveContextDocument:
			if c.save != nil {
				c.save(c.composer.Controller().Text())
			}
			return true
		case cancelContextDocument:
			if c.cancel != nil {
				c.cancel()
			}
			return true
		}
	}
	return false
}

// contextEditing opens one multiline editor at a time; opening another
// dismisses the open one.
type contextEditing struct {
	stack     *workspaceScene
	theme     kit.Theme
	glyphs    kit.Glyphs
	clipboard base.Clipboard
	notify    func(string)
	active    *contextEditorSession
}

func (e *contextEditing) open(request contextEditorRequest) *contextEditorSession {
	if e.active != nil {
		e.active.Dismiss()
	}
	editor := newContextEditor(e.theme, e.clipboard, request.Content, request.Placeholder)
	var dialog *presentationDialog
	session := &contextEditorSession{editor: editor}
	session.dismissed = func() {
		if e.active == session {
			e.active = nil
		}
		if request.Dismissed != nil {
			request.Dismissed()
		}
	}
	editor.cancel = session.Dismiss
	editor.save = func(value string) {
		if editor.phase == contextSaving || session.closed || e.active != session {
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
			if session.closed || e.active != session {
				return false
			}
			if err != nil {
				enter(contextSaveFailed, err.Error())
				return false
			}
			if editor.composer.Controller().Text() != value {
				enter(contextSavedWithNewEdits, "")
				return false
			}
			session.Dismiss()
			return true
		}
		if err := request.Save(value, complete); err != nil {
			complete(err)
			e.notify(request.Title + ": " + err.Error())
		}
	}
	dialog = newPresentationDialog(e.stack, kit.DialogConfig{Theme: e.theme, Glyphs: e.glyphs,
		Title: request.Title, Description: request.Description, Body: editor.Element(),
		Where: layout.Placement{Width: 100, Height: 24}, Keys: editor.keys,
	})
	session.dialog = dialog
	e.active = session
	dialog.Show()
	return session
}

func (e *contextEditing) dismiss() {
	if e.active != nil {
		e.active.Dismiss()
	}
}
