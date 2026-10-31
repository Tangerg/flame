package terminal

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// confirmPrompt asks the user to confirm one action at a time; a newer
// question replaces the open one.
type confirmPrompt struct {
	stack   *headless.Stack
	theme   kit.Theme
	glyphs  kit.Glyphs
	wheel   input.Wheel
	session *sessionState
	dialog  *kit.Dialog
}

// ask shows question with content, if any, and runs confirm only if the user
// chooses action while the Session the prompt belongs to is still presented.
func (c *confirmPrompt) ask(title, question, action, content string, confirm func()) {
	c.dismiss()
	generation := c.session.context
	confirmed := false
	choice := headless.NewSelect(headless.SelectConfig[bool]{Same: headless.Equal[bool], Label: question, Value: headless.Bind(&confirmed), Rows: 2})
	choice.SetOptions([]headless.Option[bool]{
		{Label: "Cancel", Value: false},
		{Label: action, Value: true},
	})
	form := headless.NewForm(choice)
	form.Keys = headless.DefaultFormKeys()
	var dialog *kit.Dialog
	dismiss := func() {
		if c.dialog == dialog {
			dialog.Controller().Dismiss()
			c.dialog = nil
		}
	}
	form.Done = func() {
		current := c.dialog == dialog
		dismiss()
		if current && confirmed && c.session.context.current(generation) {
			confirm()
		}
	}
	form.GaveUp = dismiss
	formBody := kit.NewForm(kit.FormConfig{
		Theme: c.theme, Glyphs: c.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	var body headless.Widget = formBody
	placement := layout.Placement{Width: 78, Height: 9}
	if content != "" {
		viewport := headless.NewViewport(headless.Static{Of: kit.NewParagraph(content, c.theme.Text)})
		viewport.Scroll().Wheel(c.wheel)
		body = &confirmationContentPane{viewport: viewport, form: formBody}
		placement = layout.Placement{Width: 96, Height: 26}
	}
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: c.stack, Theme: c.theme, Glyphs: c.glyphs,
		Title: title, Body: body, Where: placement,
	})
	c.dialog = dialog
	dialog.Controller().Show()
}

func (c *confirmPrompt) dismiss() {
	if c.dialog != nil {
		c.dialog.Controller().Dismiss()
		c.dialog = nil
	}
}
