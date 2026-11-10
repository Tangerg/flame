package terminal

import (
	"fmt"
	"image"
	"strings"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/fuzzy"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

// picker is the common interrupt behind sessions, models, commands and other
// searchable catalogs. Domain-specific dialogs supply only labels and actions.
type picker[T any] struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	query  *kit.Composer
	items  *base.Filter[T]
	label  func(T) string
	detail func(T) string
	pick   func(T)
	cancel func()
	group  *scene.Group

	pointerGesture pickerPointerGesture
}

type pickerPointerGesture struct {
	index   int
	active  bool
	dragged bool
}

func (p *pickerPointerGesture) begin(index int) {
	*p = pickerPointerGesture{index: index, active: true}
}

func (p *pickerPointerGesture) drag() bool {
	if !p.active {
		return false
	}
	p.dragged = true
	return true
}

func (p *pickerPointerGesture) release(index int) (pressed int, commit bool) {
	if !p.active {
		return -1, false
	}
	pressed = p.index
	commit = !p.dragged && index == pressed
	p.cancel()
	return pressed, commit
}

func (p *pickerPointerGesture) cancel() { *p = pickerPointerGesture{} }

func newPicker[T any](theme kit.Theme, glyphs kit.Glyphs, placeholder string, label, detail func(T) string, pick func(T)) *picker[T] {
	p := &picker[T]{theme: theme, glyphs: glyphs, label: label, detail: detail, pick: pick}
	p.query = kit.NewComposer(kit.ComposerConfig{Theme: theme, Glyphs: glyphs, Prompt: glyphs.Marker + " ", MaxRows: 1})
	p.query.Controller().SetPlaceholder(placeholder)
	p.items = base.NewFilter(base.FilterConfig[T]{Row: p.row})
	p.query.Controller().Session().Subscribe(func(base.ChangeMap) {
		p.pointerGesture.cancel()
		p.items.SetPattern(p.query.Controller().Text())
	})
	return p
}

func (p *picker[T]) SetItems(items []T) {
	p.pointerGesture.cancel()
	p.items.SetItems(items, func(item T) string { return strings.TrimSpace(p.label(item) + " " + p.detail(item)) })
	p.items.SetPattern(p.query.Controller().Text())
}

func (p *picker[T]) Current() (T, bool) { return p.items.Current() }

func (p *picker[T]) interruptPointerGesture() { p.pointerGesture.cancel() }

func (p *picker[T]) Reset() {
	p.query.Controller().Clear()
	p.items.SetPattern("")
	p.items.Select(0)
	p.pointerGesture.cancel()
}

func (p *picker[T]) Element() scene.Element {
	if p.group == nil {
		list := scene.NewGroup(scene.GroupConfig{
			Items: []scene.Item{{Element: p.items.Element(), Size: layout.Flex(1)}},
			Element: scene.ElementConfig{Focusable: func() bool { return true },
				FocusChanged: func(has bool) {
					if !has {
						p.pointerGesture.cancel()
					}
				},
				Preview: func(event scene.Event) scene.EventResult {
					mouse, ok := event.Input().(input.Mouse)
					if !ok {
						return scene.EventResult{}
					}
					if !event.CurrentLayout() {
						p.pointerGesture.cancel()
						return scene.EventResult{Consumed: true}
					}
					consumed := p.handleListMouse(mouse)
					return scene.EventResult{Consumed: consumed, Capture: consumed && mouse.Action == input.MouseDown, Focus: consumed && mouse.Action == input.MouseDown}
				}, Detached: func() { p.pointerGesture.cancel() }},
		})
		footer := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
			label := fmt.Sprintf("%d/%d · ↑↓ move · enter select · esc close", p.items.Matched(), p.items.Len())
			kit.Label{Text: label, Style: p.theme.Subtle, Align: layout.End, Ellipsis: p.glyphs.Ellipsis}.Draw(frame.View)
		}})
		p.group = scene.NewGroup(scene.GroupConfig{
			Axis:  layout.Down,
			Items: []scene.Item{{Element: p.query.Element(), Size: layout.Fixed(1)}, {Element: list.Element(), Size: layout.Flex(1)}, {Element: footer, Size: layout.Fixed(1)}},
			Element: scene.ElementConfig{Preview: func(event scene.Event) scene.EventResult {
				key, ok := event.Input().(input.Key)
				if !ok || !key.Down() {
					return scene.EventResult{}
				}
				p.pointerGesture.cancel()
				return scene.EventResult{Consumed: p.handleKey(key, key)}
			}},
		})
	}
	return p.group.Element()
}

func (p *picker[T]) handleKey(key input.Key, event input.Event) bool {
	if key.Code == input.Enter {
		if item, ok := p.items.Current(); ok && p.pick != nil {
			p.pick(item)
		}
		return true
	}
	if key.Code == input.Esc {
		if p.cancel != nil {
			p.cancel()
		}
		return true
	}
	for _, code := range [...]input.Code{input.Up, input.Down, input.PageUp, input.PageDown, input.Home, input.End} {
		if key.Code == code {
			return p.items.Handle(event)
		}
	}
	return false
}

func (p *picker[T]) handleListMouse(mouse input.Mouse) bool {
	switch mouse.Action {
	case input.MouseCancel:
		p.pointerGesture.cancel()
		return true
	case input.WheelUp, input.WheelDown:
		p.pointerGesture.cancel()
		return p.items.Handle(mouse)
	case input.MouseDown:
		if mouse.Button != input.ButtonLeft || !p.items.Handle(mouse) {
			p.pointerGesture.cancel()
			return false
		}
		p.pointerGesture.begin(p.items.Selected())
		return true
	case input.MouseDrag:
		if mouse.Button != input.ButtonLeft || !p.pointerGesture.drag() {
			p.pointerGesture.cancel()
			return false
		}
		p.items.Handle(mouse)
		return true
	case input.MouseUp:
		if mouse.Button != input.ButtonLeft || !p.pointerGesture.active {
			p.pointerGesture.cancel()
			return false
		}
		pressed, commit := p.pointerGesture.release(p.listIndexAt(mouse.Pos))
		if commit && p.items.Selected() == pressed {
			if item, ok := p.items.Current(); ok && p.pick != nil {
				p.pick(item)
			}
		}
		return true
	default:
		return p.items.Handle(mouse)
	}
}

func (p *picker[T]) listIndexAt(point image.Point) int {
	index, ok := p.items.Hit(point)
	if !ok {
		return -1
	}
	return index
}

func (p *picker[T]) row(view grid.View, _ int, item T, _ fuzzy.Match, selected bool) {
	width, _ := view.Size()
	if width <= 0 {
		return
	}
	base := p.theme.Text
	prefix := "  "
	if selected {
		base = base.Merge(p.theme.Selection)
		view.Fill(view.Bounds(), p.theme.Selection)
		prefix = p.glyphs.Marker + " "
	}
	label := p.label(item)
	detail := strings.TrimSpace(p.detail(item))
	detailWidth := text.Width(detail)
	prefixWidth := text.Width(prefix)
	labelWidth := max(width-prefixWidth, 1)
	if detail != "" && detailWidth+prefixWidth+3 < width {
		labelWidth = width - detailWidth - prefixWidth - 3
		view.Text(width-detailWidth, 0, detail, base.Merge(p.theme.Subtle))
	}
	shown := text.Truncate(label, max(labelWidth, 1), p.glyphs.Ellipsis)
	view.Text(0, 0, prefix, base.Merge(p.theme.Accent))
	view.Text(prefixWidth, 0, shown, base)
}
