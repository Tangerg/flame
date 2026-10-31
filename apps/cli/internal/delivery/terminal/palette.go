package terminal

import (
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/extensions"
	"github.com/Tangerg/oolong/components/headless"
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
