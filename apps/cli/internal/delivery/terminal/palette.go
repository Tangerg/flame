package terminal

import (
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/extensions"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

type commandPaletteItem struct {
	command      headless.Command
	category     string
	availability extensions.CommandAvailability
}

func (a *app) chooseCommand(item commandPaletteItem) {
	command := item.command
	if !item.availability.Enabled() {
		a.message("/" + command.Name + " unavailable: " + item.availability.Reason())
		return
	}
	if a.commands.arguments(command.Name).TakesInput() {
		a.composer.Editor().SetText("/" + command.Name + " ")
		a.composer.Editor().Focus(true)
		a.composer.scheduleDraftPersistence()
		return
	}
	a.runCommand(command.Name, "", func() {})
}

func (a *app) showCommandPalette() {
	found := a.commands.find("")
	commands := make([]commandPaletteItem, 0, len(found))
	for _, match := range found {
		commands = append(commands, commandPaletteItem{
			command: match.Command, category: a.commands.category(match.Command.Name),
			availability: a.commands.availability(match.Command.Name, a),
		})
	}
	slices.SortStableFunc(commands, func(left, right commandPaletteItem) int {
		if compared := commandCategoryRank(left.category) - commandCategoryRank(right.category); compared != 0 {
			return compared
		}
		return strings.Compare(left.command.Name, right.command.Name)
	})
	a.palette.show(commands)
	a.status.note("choose a command")
}

// commandCategoryRank keeps the commands used to navigate and understand the
// current conversation ahead of less frequent configuration commands. The
// category label remains presentation data; ordering is an explicit product
// policy rather than an accidental consequence of English spelling.
func commandCategoryRank(category string) int {
	switch category {
	case commandCategoryApplication:
		return 0
	case commandCategoryTranscript:
		return 1
	case commandCategorySessions:
		return 2
	case commandCategoryComposer:
		return 3
	case commandCategoryRuntime:
		return 4
	case commandCategoryExtensions:
		return 5
	default:
		return 6
	}
}

// searchPrompt asks for text to find in the live transcript.
type searchPrompt struct {
	dialog *presentationDialog
	query  string
}

func newSearchPrompt(stack *headless.Stack, theme kit.Theme, glyphs kit.Glyphs, find func(string)) *searchPrompt {
	p := &searchPrompt{}
	field := headless.NewText(headless.TextConfig{Label: "Find in the live transcript", Placeholder: "text", Value: headless.Bind(&p.query), Check: requiredText})
	form := headless.NewForm(field)
	form.Keys = headless.DefaultFormKeys()
	form.Done = func() {
		if !p.dialog.Open() {
			return
		}
		p.dialog.Dismiss()
		find(p.query)
	}
	form.GaveUp = func() { p.dialog.Dismiss() }
	dressed := kit.NewForm(kit.FormConfig{
		Theme: theme, Glyphs: glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	p.dialog = newPresentationDialog(kit.DialogConfig{
		Stack: stack, Theme: theme, Glyphs: glyphs, Title: "Search", Body: dressed,
		Where: layout.Placement{Width: 68, Height: 7},
	})
	return p
}

func (p *searchPrompt) show() {
	p.query = ""
	p.dialog.Show()
}
