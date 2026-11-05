package terminal

import (
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

// confirmationContentPane keeps the complete reviewed content scrollable while
// the decision remains visible. Arrow keys choose; page keys and the wheel read.
type confirmationContentPane struct {
	viewport *base.Viewport
	form     *kit.Form
	group    *scene.Group
}

func (p *confirmationContentPane) Element() scene.Element {
	if p.group == nil {
		// Decisions own arrow-key focus; the reviewed document remains readable
		// through the wheel and the pane's page-key actions.
		if err := p.viewport.Element().SetFocusable(func() bool { return false }); err != nil {
			panic(err)
		}
		p.group = scene.NewGroup(scene.GroupConfig{
			Axis:  layout.Down,
			Items: []scene.Item{{Element: p.viewport.Element(), Size: layout.Flex(1)}, {Element: p.form.Element(), Size: layout.Measured(0, 7)}},
			Element: scene.ElementConfig{Preview: func(event scene.Event) scene.EventResult {
				key, ok := event.Input().(input.Key)
				if !ok || !key.Down() {
					return scene.EventResult{}
				}
				switch key.Code {
				case input.PageUp:
					return scene.EventResult{Consumed: p.viewport.Do(base.ScrollPageUp)}
				case input.PageDown:
					return scene.EventResult{Consumed: p.viewport.Do(base.ScrollPageDown)}
				}
				return scene.EventResult{}
			}},
		})
	}
	return p.group.Element()
}
