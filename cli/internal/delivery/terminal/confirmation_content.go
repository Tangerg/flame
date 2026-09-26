package terminal

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

// confirmationContentPane keeps the complete reviewed content scrollable while
// the decision remains visible. Arrow keys choose; page keys and the wheel read.
type confirmationContentPane struct {
	viewport       *headless.Viewport
	form           *kit.Form
	viewportRegion headless.PointerRegion
	formRegion     headless.PointerRegion
}

func (p *confirmationContentPane) Draw(frame headless.Frame) {
	rects := (layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(min(p.form.HeightForWidth(frame.Bounds().Dx()), 7))},
	})
	p.viewportRegion.Stage(frame, rects[0], p.viewport)
	p.formRegion.Stage(frame, rects[1], p.form)
	rows := frame.Subs(rects)
	p.viewport.Draw(rows[0])
	p.form.Draw(rows[1])
}

func (p *confirmationContentPane) Handle(event input.Event) bool {
	if mouse, ok := event.(input.Mouse); ok {
		if handled, delivered := p.formRegion.Handle(mouse); delivered {
			return handled
		}
		handled, _ := p.viewportRegion.Handle(mouse)
		return handled
	}
	if key, ok := event.(input.Key); ok && key.Down() && (key.Chord().Code == input.PageUp || key.Chord().Code == input.PageDown) {
		return p.viewport.Handle(event)
	}
	if p.form.Handle(event) {
		return true
	}
	return p.viewport.Handle(event)
}

func (p *confirmationContentPane) Focus(has bool) {
	p.form.Focus(has)
	p.viewport.Focus(has)
}
