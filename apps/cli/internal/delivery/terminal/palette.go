package terminal

import (
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/extensions"
	"github.com/Tangerg/oolong/ui/base"
)

type commandPaletteItem struct {
	command      base.Command
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
		a.composer.Controller().SetText("/" + command.Name + " ")
		a.shell.FocusPrompt()
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

// Built-in categories in palette order; extension-defined categories follow.
var commandCategoryOrder = []string{
	commandCategoryApplication,
	commandCategoryTranscript,
	commandCategorySessions,
	commandCategoryComposer,
	commandCategoryRuntime,
	commandCategoryContext,
	commandCategoryConnections,
	commandCategoryWorkspace,
	commandCategoryExtensions,
}

func commandCategoryRank(category string) int {
	if rank := slices.Index(commandCategoryOrder, category); rank >= 0 {
		return rank
	}
	return len(commandCategoryOrder)
}
