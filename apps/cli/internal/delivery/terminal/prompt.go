package terminal

import (
	"image"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

type promptView struct {
	theme             kit.Theme
	box               kit.Box
	composer          *kit.Composer
	help              kit.Help
	rows              *scene.Group
	keys              *keymap.Map
	busyKeys          *keymap.Map
	busyQueuedKeys    *keymap.Map
	busy              bool
	queued            int
	transcriptFocused bool
	selection         transcriptSelection
	transcriptKeys    *keymap.Map
	compact           bool
}

func newPromptView(
	theme kit.Theme,
	glyphs kit.Glyphs,
	keys *keymap.Map,
	composer *kit.Composer,
	options authoring.RunOptions,
) *promptView {
	p := &promptView{
		theme: theme, composer: composer,
		box:  kit.Box{Theme: theme, Glyphs: glyphs, Padding: layout.Symmetric(0, 1), FooterAlign: layout.End},
		help: kit.Help{Theme: theme, Keys: keys, Separator: "  " + glyphs.Vertical + "  "},
		keys: keys, busyKeys: remapHelpAction(keys, sendPrompt, queueFollowUp),
		busyQueuedKeys: remapHelpAction(keys, sendPrompt, queueOrSendNext),
	}
	help := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { p.help.Draw(frame.View) }})
	p.rows = scene.NewGroup(scene.GroupConfig{
		Items: []scene.Item{{Element: composer.Element()}, {Element: help}},
		Element: scene.ElementConfig{
			Measure: func(axis layout.Axis, width int) int {
				if axis == layout.Down {
					return p.HeightForWidth(width)
				}
				return 0
			},
			Paint: func(frame scene.Frame) {
				if p.isCompact(frame.Bounds().Size()) {
					return
				}
				box := p.box
				if p.Element().FocusedWithin() {
					box.Theme.Border = p.theme.Accent
				}
				box.Draw(frame.Sub(grid.Area(0, 0, frame.Bounds().Dx(), max(frame.Bounds().Dy()-1, 0))).View)
			},
		},
		Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
			if p.isCompact(space) {
				return []image.Rectangle{{Max: space}, {}}
			}
			panelSize := image.Pt(space.X, max(space.Y-1, 0))
			return []image.Rectangle{p.box.InnerRect(panelSize), grid.Area(0, panelSize.Y, space.X, 1)}
		},
	})
	p.SetOptions(options)
	p.SetBusy(false)
	return p
}

func (p *promptView) Element() scene.Element { return p.rows.Element() }

func (p *promptView) isCompact(size image.Point) bool { return p.compact || size.Y <= 1 }

func (p *promptView) HeightForWidth(width int) int {
	if p.compact {
		return 1
	}
	overhead := p.box.Overhead()
	return layout.Sum(p.composer.HeightForWidth(max(width-overhead.X, 0)), overhead.Y, 1)
}

func (p *promptView) SetCompact(compact bool) {
	p.compact = compact
	p.Element().Invalidate()
}

func (p *promptView) SetOptions(options authoring.RunOptions) {
	p.box.Footer = modelLabel(options)
}

func (p *promptView) SetPendingKeySequence(hint string) {
	p.box.Title = hint
}

func (p *promptView) SetBusy(busy bool) {
	p.busy = busy
	p.refreshHelp()
}

func (p *promptView) SetQueued(count int) {
	p.queued = max(count, 0)
	p.refreshHelp()
}

func (p *promptView) SetTranscriptFocused(focused bool) {
	p.transcriptFocused = focused
	p.refreshHelp()
}

func (p *promptView) SetTranscriptSelection(selection transcriptSelection) {
	p.selection = selection
	p.refreshHelp()
}

func (p *promptView) SetTranscriptKeys(keys *keymap.Map) {
	p.transcriptKeys = keys
	p.refreshHelp()
}

func (p *promptView) refreshHelp() {
	p.help.Keys = p.keys
	switch {
	case p.transcriptFocused:
		p.showTranscriptHelp()
		return
	case p.busy:
		p.showBusyHelp()
		return
	}
	p.help.Show = []keymap.Action{sendPrompt, insertNewline, editPrompt, commandPalette, showSessions, chooseModel}
	if p.queued > 0 {
		p.help.Show = append(p.help.Show, manageQueue)
	}
}

func (p *promptView) showTranscriptHelp() {
	p.help.Keys = p.transcriptKeys
	if !p.selection.Present {
		p.help.Show = []keymap.Action{transcriptPrompt}
		return
	}
	p.help.Show = []keymap.Action{base.SelectPrev, base.SelectNext, transcriptPrompt}
	if p.selection.Expandable {
		action := base.Expand
		if p.selection.Expanded {
			action = base.Collapse
		}
		p.help.Show = append(p.help.Show, action, toggleDetails)
	}
	if p.selection.Readable {
		p.help.Show = append(p.help.Show, openReader)
	}
	p.help.Show = append(p.help.Show, base.Copy)
}

func (p *promptView) showBusyHelp() {
	p.help.Keys = p.busyKeys
	p.help.Show = []keymap.Action{queueFollowUp, cancelRun, insertNewline, toggleDetails}
	if p.queued == 0 {
		return
	}
	p.help.Keys = p.busyQueuedKeys
	p.help.Show = []keymap.Action{queueOrSendNext, cancelRun, insertNewline, manageQueue, toggleDetails}
}

func remapHelpAction(keys *keymap.Map, from, to keymap.Action) *keymap.Map {
	remapped := &keymap.Map{}
	for _, binding := range keys.Bindings() {
		action := binding.Action
		if action == from {
			action = to
		}
		remapped.Bind(action, binding.Keys...)
	}
	return remapped
}
