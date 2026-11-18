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
	viewport.SetKeys(guideKeys)
	viewport.Scroll().Wheel(wheel)
	return &shortcutGuide{viewport: viewport, dialog: newPresentationDialog(stack, kit.DialogConfig{Theme: theme, Glyphs: glyphs, Title: "Shortcuts", Body: viewport.Element(),
		Where: layout.Placement{Width: 88, Height: 24}, Keys: guideKeys,
	})}
}

func (g *shortcutGuide) show() {
	g.viewport.Scroll().ToTop()
	g.dialog.Show()
}

// The guide's own keys sit beside the command categories they document.
const shortcutAreaGuide = "Guide"

func collectShortcutRows(applicationKeys, transcriptKeys, guideKeys *keymap.Map) []shortcutRow {
	definitions := []shortcutDefinition{
		{area: shortcutAreaGuide, action: base.ScrollUp, description: "scroll this guide up", keys: guideKeys},
		{area: shortcutAreaGuide, action: base.ScrollDown, description: "scroll this guide down", keys: guideKeys},
		{area: shortcutAreaGuide, action: base.Close, description: "close this guide", keys: guideKeys},
		{area: commandCategoryComposer, action: sendPrompt, description: "send prompt or queue follow-up", keys: applicationKeys},
		{area: commandCategoryComposer, action: insertNewline, description: "insert a newline", keys: applicationKeys},
		{area: commandCategoryComposer, action: cancelRun, description: "clear draft or cancel active run", keys: applicationKeys},
		{area: commandCategoryComposer, action: historyPrevious, description: "recall previous prompt", keys: applicationKeys},
		{area: commandCategoryComposer, action: historyNext, description: "recall next prompt", keys: applicationKeys},
		{area: commandCategoryComposer, action: editPrompt, description: "edit prompt in external editor", keys: applicationKeys},
		{area: commandCategoryComposer, action: chooseModel, description: "choose provider and model", keys: applicationKeys},
		{area: commandCategoryApplication, action: commandPalette, description: "open command palette", keys: applicationKeys},
		{area: commandCategoryApplication, action: showShortcuts, description: "open this shortcut guide", keys: applicationKeys},
		{area: commandCategoryApplication, action: showSessions, description: "search and switch sessions", keys: applicationKeys},
		{area: commandCategoryApplication, action: showTimeline, description: "inspect runs and live subagents", keys: applicationKeys},
		{area: commandCategoryApplication, action: searchTranscript, description: "find text in the live transcript", keys: applicationKeys},
		{area: commandCategoryApplication, action: manageQueue, description: "manage queued follow-ups", keys: applicationKeys},
		{area: commandCategoryApplication, action: toggleDetails, description: "expand or collapse all tool details", keys: applicationKeys},
		{area: commandCategoryApplication, action: nextMatch, description: "go to next search match", keys: applicationKeys},
		{area: commandCategoryApplication, action: previousMatch, description: "go to previous search match", keys: applicationKeys},
		{area: commandCategoryApplication, action: quitApp, description: "quit after confirmation", keys: applicationKeys},
		{area: commandCategoryTranscript, action: commandPalette, description: "open command palette", keys: transcriptKeys},
		{area: commandCategoryTranscript, action: scrollPageUp, description: "scroll one page up", keys: applicationKeys},
		{area: commandCategoryTranscript, action: scrollPageDown, description: "scroll one page down", keys: applicationKeys},
		{area: commandCategoryTranscript, action: scrollTop, description: "jump to the beginning", keys: applicationKeys},
		{area: commandCategoryTranscript, action: scrollBottom, description: "jump to the latest output", keys: applicationKeys},
		{area: commandCategoryTranscript, action: base.SelectPrev, description: "select previous entry", keys: transcriptKeys},
		{area: commandCategoryTranscript, action: base.SelectNext, description: "select next entry", keys: transcriptKeys},
		{area: commandCategoryTranscript, action: base.SelectFirst, description: "select first retained entry", keys: transcriptKeys},
		{area: commandCategoryTranscript, action: base.SelectLast, description: "select last retained entry", keys: transcriptKeys},
		{area: commandCategoryTranscript, action: base.Expand, description: "expand selected tool", keys: transcriptKeys},
		{area: commandCategoryTranscript, action: base.Collapse, description: "collapse selected tool", keys: transcriptKeys},
		{area: commandCategoryTranscript, action: toggleDetails, description: "toggle selected tool", keys: transcriptKeys},
		{area: commandCategoryTranscript, action: base.Copy, description: "copy selected entry", keys: transcriptKeys},
		{area: commandCategoryTranscript, action: transcriptPrompt, description: "return to the composer", keys: transcriptKeys},
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
