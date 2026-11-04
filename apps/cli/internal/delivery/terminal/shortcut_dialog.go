package terminal

import (
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/kit"
)

type shortcutRow struct {
	area        string
	bindings    string
	description string
}

type shortcutDefinition struct {
	area        string
	action      keymap.Action
	description string
	keys        *keymap.Map
}

// shortcutGuide shows every configured key binding.
type shortcutGuide struct {
	viewport *base.Viewport
	dialog   *presentationDialog
}

func newShortcutGuide(stack *workspaceScene, theme kit.Theme, glyphs kit.Glyphs, wheel input.Wheel, applicationKeys, transcriptKeys *keymap.Map) *shortcutGuide {
	guideKeys := base.DefaultScrollKeys()
	for _, binding := range base.DefaultDialogKeys().Keys(base.Close) {
		guideKeys.Bind(base.Close, binding...)
	}
	rows := collectShortcutRows(applicationKeys, transcriptKeys, guideKeys)
	table := &kit.Table{
		Theme: theme,
		Columns: []kit.Column{
			{Title: "Area", Size: layout.Measured(4, 12)},
			{Title: "Keys", Size: layout.Measured(8, 28)},
			{Title: "Action", Size: layout.Flex(1)},
		},
		Rows:   len(rows),
		Header: true,
		Gap:    2,
		Cell: func(row, column int) kit.Cell {
			entry := rows[row]
			values := [...]string{entry.area, entry.bindings, entry.description}
			styles := [...]grid.Style{theme.Subtle, theme.Accent, theme.Text}
			return kit.LabelCell(kit.Label{Text: values[column], Style: styles[column], Ellipsis: glyphs.Ellipsis})
		},
	}
	viewport := base.NewViewport(base.ViewportConfig{Content: passiveElement(table)})
	viewport.Keys = guideKeys
	viewport.Scroll().Wheel(wheel)
	return &shortcutGuide{viewport: viewport, dialog: newPresentationDialog(stack, kit.DialogConfig{Theme: theme, Glyphs: glyphs, Title: "Shortcuts", Body: viewport.Element(),
		Where: layout.Placement{Width: 88, Height: 24}, Keys: guideKeys,
	})}
}

func (g *shortcutGuide) show() {
	g.viewport.Scroll().ToTop()
	g.dialog.Show()
}

func collectShortcutRows(applicationKeys, transcriptKeys, guideKeys *keymap.Map) []shortcutRow {
	definitions := []shortcutDefinition{
		{area: "Guide", action: base.ScrollUp, description: "scroll this guide up", keys: guideKeys},
		{area: "Guide", action: base.ScrollDown, description: "scroll this guide down", keys: guideKeys},
		{area: "Guide", action: base.Close, description: "close this guide", keys: guideKeys},
		{area: "Composer", action: sendPrompt, description: "send prompt or queue follow-up", keys: applicationKeys},
		{area: "Composer", action: insertNewline, description: "insert a newline", keys: applicationKeys},
		{area: "Composer", action: cancelRun, description: "clear draft or cancel active run", keys: applicationKeys},
		{area: "Composer", action: historyPrevious, description: "recall previous prompt", keys: applicationKeys},
		{area: "Composer", action: historyNext, description: "recall next prompt", keys: applicationKeys},
		{area: "Composer", action: editPrompt, description: "edit prompt in external editor", keys: applicationKeys},
		{area: "Composer", action: chooseModel, description: "choose provider and model", keys: applicationKeys},
		{area: "Application", action: commandPalette, description: "open command palette", keys: applicationKeys},
		{area: "Application", action: showShortcuts, description: "open this shortcut guide", keys: applicationKeys},
		{area: "Application", action: showSessions, description: "search and switch sessions", keys: applicationKeys},
		{area: "Application", action: showTimeline, description: "inspect runs and live subagents", keys: applicationKeys},
		{area: "Application", action: searchTranscript, description: "find text in the live transcript", keys: applicationKeys},
		{area: "Application", action: manageQueue, description: "manage queued follow-ups", keys: applicationKeys},
		{area: "Application", action: toggleDetails, description: "expand or collapse all tool details", keys: applicationKeys},
		{area: "Application", action: nextMatch, description: "go to next search match", keys: applicationKeys},
		{area: "Application", action: previousMatch, description: "go to previous search match", keys: applicationKeys},
		{area: "Application", action: quitApp, description: "quit after confirmation", keys: applicationKeys},
		{area: "Transcript", action: commandPalette, description: "open command palette", keys: transcriptKeys},
		{area: "Transcript", action: scrollPageUp, description: "scroll one page up", keys: applicationKeys},
		{area: "Transcript", action: scrollPageDown, description: "scroll one page down", keys: applicationKeys},
		{area: "Transcript", action: scrollTop, description: "jump to the beginning", keys: applicationKeys},
		{area: "Transcript", action: scrollBottom, description: "jump to the latest output", keys: applicationKeys},
		{area: "Transcript", action: base.SelectPrev, description: "select previous entry", keys: transcriptKeys},
		{area: "Transcript", action: base.SelectNext, description: "select next entry", keys: transcriptKeys},
		{area: "Transcript", action: base.SelectFirst, description: "select first retained entry", keys: transcriptKeys},
		{area: "Transcript", action: base.SelectLast, description: "select last retained entry", keys: transcriptKeys},
		{area: "Transcript", action: base.Expand, description: "expand selected tool", keys: transcriptKeys},
		{area: "Transcript", action: base.Collapse, description: "collapse selected tool", keys: transcriptKeys},
		{area: "Transcript", action: toggleDetails, description: "toggle selected tool", keys: transcriptKeys},
		{area: "Transcript", action: base.Copy, description: "copy selected entry", keys: transcriptKeys},
		{area: "Transcript", action: transcriptPrompt, description: "return to the composer", keys: transcriptKeys},
	}

	rows := make([]shortcutRow, 0, len(definitions))
	for _, definition := range definitions {
		bindings := formatKeyBindings(definition.keys, definition.action, " / ")
		if bindings == "" {
			continue
		}
		rows = append(rows, shortcutRow{
			area: definition.area, bindings: bindings, description: definition.description,
		})
	}
	return rows
}
