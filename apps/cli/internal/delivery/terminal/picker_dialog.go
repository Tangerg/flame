package terminal

import (
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/kit"
)

// pickerDialog is a searchable list in a modal. Choosing an item closes the
// modal before choose runs, so choose sees the picker already dismissed;
// dismissed runs whenever the modal is closed without a choice.
type pickerDialog[T any] struct {
	picker *picker[T]
	dialog *presentationDialog
}

type pickerDialogConfig[T any] struct {
	stack     *workspaceScene
	theme     kit.Theme
	glyphs    kit.Glyphs
	title     string
	search    string
	where     layout.Placement
	label     func(T) string
	detail    func(T) string
	choose    func(T)
	dismissed func()
}

func newPickerDialog[T any](cfg pickerDialogConfig[T]) *pickerDialog[T] {
	d := &pickerDialog[T]{}
	d.picker = newPicker(cfg.theme, cfg.glyphs, cfg.search, cfg.label, cfg.detail, func(item T) {
		if !d.dialog.Open() {
			return
		}
		d.dismiss(cfg.dismissed)
		cfg.choose(item)
	})
	d.dialog = newPresentationDialog(cfg.stack, kit.DialogConfig{Theme: cfg.theme, Glyphs: cfg.glyphs, Title: cfg.title, Body: d.picker.Element(), Where: cfg.where})
	d.picker.cancel = func() { d.dismiss(cfg.dismissed) }
	return d
}

func (d *pickerDialog[T]) dismiss(dismissed func()) {
	if dismissed != nil {
		dismissed()
	}
	d.dialog.Dismiss()
}

// show opens the picker over items, clearing any previous search.
func (d *pickerDialog[T]) show(items []T) {
	d.picker.Reset()
	d.picker.SetItems(items)
	d.dialog.Show()
}
