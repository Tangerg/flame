package terminal

import (
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

// confirmPrompt asks the user to confirm one action at a time; a newer
// question replaces the open one.
type confirmPrompt struct {
	stack   *workspaceScene
	theme   kit.Theme
	glyphs  kit.Glyphs
	wheel   input.Wheel
	session *sessionState
	dialog  *presentationDialog
}

// ask shows question with content, if any, and runs confirm only if the user
// chooses action while the Session the prompt belongs to is still presented.
func (c *confirmPrompt) ask(title, question, action, content string, confirm func()) {
	c.dismiss()
	generation := c.session.context
	choice := base.NewSelect(base.SelectConfig[bool]{
		Same: base.Equal[bool], Label: question, Rows: 2,
		Options: []base.Option[bool]{{Label: "Cancel", Value: false}, {Label: action, Value: true}},
	})
	var dialog *presentationDialog
	dismiss := func() {
		if c.dialog == dialog {
			dialog.Dismiss()
			c.dialog = nil
		}
	}
	formBody := kit.NewForm(kit.FormConfig{
		Theme: c.theme, Glyphs: c.glyphs, Fields: []base.Field{choice}, Keys: base.DefaultFormKeys(),
		Hints: []keymap.Action{base.Submit, base.Cancel},
		Submit: func(base.SubmissionID) (scene.ActionState, error) {
			if c.dialog != dialog {
				return scene.Unchanged, scene.ErrStale
			}
			selected, ok := choice.Chosen()
			if !ok {
				return scene.Unchanged, base.ErrInvalidValue
			}
			dismiss()
			if selected.Value && c.session.context.current(generation) {
				confirm()
			}
			return scene.Applied, nil
		},
	})
	body := formBody.Element()
	placement := layout.Placement{Width: 78, Height: 9}
	if content != "" {
		viewport := base.NewViewport(base.ViewportConfig{Content: passiveElement(kit.NewParagraph(content, c.theme.Text))})
		viewport.Scroll().Wheel(c.wheel)
		pane := &confirmationContentPane{viewport: viewport, form: formBody}
		body = pane.Element()
		placement = layout.Placement{Width: 96, Height: 26}
	}
	dialog = newPresentationDialog(c.stack, kit.DialogConfig{
		Theme: c.theme, Glyphs: c.glyphs, Title: title, Body: formNavigation(body, dismiss), Where: placement,
	})
	c.dialog = dialog
	dialog.Show()
}

func (c *confirmPrompt) dismiss() {
	if c.dialog != nil {
		c.dialog.Dismiss()
		c.dialog = nil
	}
}
