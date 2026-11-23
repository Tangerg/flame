package terminal

import (
	"fmt"
	"image"
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/fuzzy"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

const pickerDetailGap = 3

type picker[T any] struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	query  *kit.Editor
	items  *base.Filter[T]
	label  func(T) string
	detail func(T) string
	pick   func(T)
	cancel func()
	group  *scene.Group

	press pointerPress[int]
}

func newPicker[T any](theme kit.Theme, glyphs kit.Glyphs, placeholder string, label, detail func(T) string, pick func(T)) *picker[T] {
	p := &picker[T]{theme: theme, glyphs: glyphs, label: label, detail: detail, pick: pick}
	p.query = kit.NewEditor(kit.EditorConfig{
		Behavior: base.EditorConfig{
			MaxRows: 1,
		},
		Theme:  theme,
		Glyphs: glyphs,
		Prompt: glyphs.Marker + " ",
	})
	p.query.Controller().SetPlaceholder(placeholder)
	p.items = base.NewFilter(base.FilterConfig[T]{Row: p.row})
	p.query.Controller().Session().Subscribe(func(base.ChangeMap) {
		p.press.cancel()
		p.items.SetPattern(p.query.Controller().Text())
	})
	return p
}

func (p *picker[T]) SetItems(items []T) {
	p.press.cancel()
	p.items.SetItems(items, func(item T) string { return strings.TrimSpace(p.label(item) + " " + p.detail(item)) })
	p.items.SetPattern(p.query.Controller().Text())
}

func (p *picker[T]) Current() (T, bool) { return p.items.Current() }

func (p *picker[T]) cancelPress() { p.press.cancel() }

func (p *picker[T]) Reset() {
	p.query.Controller().Clear()
	p.items.SetPattern("")
	p.items.Select(0)
	p.press.cancel()
}

func (p *picker[T]) Element() scene.Element {
	if p.group == nil {
		list := scene.NewGroup(scene.GroupConfig{
			Items: []scene.Item{{Element: p.items.Element(), Size: layout.Flex(1)}},
			Element: scene.ElementConfig{Focusable: func() bool { return true },
				FocusChanged: func(has bool) {
					if !has {
						p.press.cancel()
					}
				},
				Preview: func(event scene.Event) scene.EventResult {
					mouse, ok := event.Input().(input.Mouse)
					if !ok {
						return scene.EventResult{}
					}
					if !event.CurrentLayout() {
						p.press.cancel()
						return scene.EventResult{Consumed: true}
					}
					consumed := p.handleListMouse(mouse)
					return scene.EventResult{Consumed: consumed, Capture: consumed && mouse.Action == input.MouseDown, Focus: consumed && mouse.Action == input.MouseDown}
				}, Detached: func() { p.press.cancel() }},
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
				p.press.cancel()
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
		p.press.cancel()
		return true
	case input.WheelUp, input.WheelDown:
		p.press.cancel()
		return p.items.Handle(mouse)
	case input.MouseDown:
		if mouse.Button != input.ButtonLeft || !p.items.Handle(mouse) {
			p.press.cancel()
			return false
		}
		p.press.begin(p.items.Selected())
		return true
	case input.MouseDrag:
		if mouse.Button != input.ButtonLeft || !p.press.drag() {
			p.press.cancel()
			return false
		}
		p.items.Handle(mouse)
		return true
	case input.MouseUp:
		return p.releaseListPress(mouse)
	default:
		return p.items.Handle(mouse)
	}
}

func (p *picker[T]) releaseListPress(mouse input.Mouse) bool {
	if _, held := p.press.pressed(); mouse.Button != input.ButtonLeft || !held {
		p.press.cancel()
		return false
	}
	pressed, click := p.press.release()
	if !click || p.listIndexAt(mouse.Pos) != pressed || p.items.Selected() != pressed {
		return true
	}
	if item, ok := p.items.Current(); ok && p.pick != nil {
		p.pick(item)
	}
	return true
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
	if detail != "" && detailWidth+prefixWidth+pickerDetailGap < width {
		labelWidth = width - detailWidth - prefixWidth - pickerDetailGap
		view.Text(width-detailWidth, 0, detail, base.Merge(p.theme.Subtle))
	}
	shown := text.Of(label, base).Truncate(max(labelWidth, 1), p.glyphs.Ellipsis)
	view.Text(0, 0, prefix, base.Merge(p.theme.Accent))
	shown.Draw(view, prefixWidth, 0)
}
