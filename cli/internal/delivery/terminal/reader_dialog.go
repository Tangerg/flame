package terminal

import (
	"context"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

func (a *app) buildReader(theme kit.Theme, glyphs kit.Glyphs) {
	a.dialogs.reader = newReaderPane(theme, glyphs, a.syntax, a.loop.Environment().Wheel(), a.loop.Clipboard())
	a.dialogs.readerDialog = newPresentationDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Reader", Body: a.dialogs.reader,
		Where: layout.Placement{},
	})
	a.dialogs.reader.dismiss = a.dismissReader
	a.dialogs.reader.openSearch = func() {
		if a.dialogs.readerDialog.Open() {
			a.showReaderSearchDialog()
		}
	}
	a.dialogs.reader.onCopied = func() { a.status.note("copied reader text") }

	field := &headless.Text{Label: "Find in the reader", Placeholder: "text", Value: headless.Bind(&a.dialogs.readerSearchQuery), Check: requiredText}
	form := headless.NewForm(field)
	form.Keys = headless.DefaultFormKeys()
	form.Done = func() {
		if !a.dialogs.readerDialog.Open() || !a.dialogs.readerSearchDialog.Open() {
			return
		}
		a.dialogs.readerSearchDialog.Dismiss()
		a.dialogs.reader.Find(a.dialogs.readerSearchQuery)
	}
	form.GaveUp = func() { a.dialogs.readerSearchDialog.Dismiss() }
	dressed := kit.NewForm(kit.FormConfig{
		Theme: theme, Glyphs: glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	a.dialogs.readerSearchDialog = newPresentationDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Search reader", Body: dressed,
		Where: layout.Placement{Width: 68, Height: 7},
	})
	a.listenForReaderSearch()
}

func (a *app) dismissReader() {
	a.operations.Cancel(readerDocumentOperation)
	a.dialogs.workspaceReader = workspaceReaderNone
	a.setRuntimeReader(runtimeReaderNone)
	if a.dialogs.reader != nil {
		a.dialogs.reader.CloseDocument()
	}
	if a.dialogs.readerSearchDialog != nil {
		a.dialogs.readerSearchDialog.Dismiss()
	}
	if a.dialogs.readerDialog != nil {
		a.dialogs.readerDialog.Dismiss()
	}
}

func (a *app) OpenReader() {
	target, ok := a.transcript.selectedReaderTarget()
	if !ok {
		a.status.note("select a readable transcript entry")
		return
	}
	a.dialogs.workspaceReader = workspaceReaderNone
	a.setRuntimeReader(runtimeReaderNone)
	a.openReaderTarget(target)
}

func (a *app) openReaderDocument(document readerDocument) {
	a.openReaderTarget(readerTarget{document: document})
}

func (a *app) openReaderTarget(target readerTarget) {
	a.operations.Cancel(readerDocumentOperation)
	a.dialogs.reader.Open(target)
	a.dialogs.readerDialog.Controller().SetDescription(target.document.Title)
	a.dialogs.readerDialog.Show()
}

func (a *app) showReaderSearchDialog() {
	a.dialogs.readerSearchQuery = ""
	a.dialogs.readerSearchDialog.Show()
}

func (a *app) listenForReaderSearch() {
	results := a.dialogs.reader.SearchResults()
	dispatcher := a.loop.Dispatcher()
	a.operations.Go(readerSearchOperation, true, func(ctx context.Context, lease operationLease) {
		for {
			select {
			case result, ok := <-results:
				if !ok {
					return
				}
				if err := post(ctx, dispatcher, func() {
					if a.operations.Current(lease) && !a.closed {
						a.dialogs.reader.AcceptSearch(result)
					}
				}); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	})
}

var _ headless.Widget = (*readerPane)(nil)
