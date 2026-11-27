package terminal

import (
	"github.com/Tangerg/flame/apps/cli/internal/delivery/format"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

const sessionPreviewRows = 4

type sessionCenterPane struct {
	group          *scene.Group
	theme          kit.Theme
	glyphs         kit.Glyphs
	picker         *picker[conversation.Session]
	catalog        sessionCatalog
	loadMore       func()
	toggleFavorite func(conversation.Session)
	rename         func(conversation.Session)
	delete         func(conversation.Session)
}

func newSessionCenterPane(theme kit.Theme, glyphs kit.Glyphs, open func(conversation.Session)) *sessionCenterPane {
	center := &sessionCenterPane{theme: theme, glyphs: glyphs}
	center.picker = newPicker(theme, glyphs, pickerConfig[conversation.Session]{
		placeholder: "search loaded sessions",
		label: func(session conversation.Session) string {
			group := "Recent"
			if session.Favorite {
				group = glyphs.Taken + " Favorites"
			}
			return group + format.Separator + displayTitle(session)
		},
		detail: func(session conversation.Session) string { return compactRelativeAge(session.UpdatedAt) },
		same:   func(left, right conversation.Session) bool { return left.ID == right.ID },
		choose: open,
	})
	return center
}

func (s *sessionCenterPane) Reset() {
	s.picker.Reset()
	s.SetCatalog(sessionCatalog{})
}

func (s *sessionCenterPane) SetCatalog(catalog sessionCatalog) {
	s.catalog = catalog
	s.picker.SetItems(catalog.items)
}

func (s *sessionCenterPane) Element() scene.Element {
	if s.group == nil {
		preview := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { s.drawPreview(frame.View) }})
		help := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
			more := "all sessions loaded"
			if s.catalog.nextCursor != "" {
				more = "alt+l load more"
			}
			kit.Label{Text: "alt+f favorite · alt+r rename · alt+d delete · " + more, Style: s.theme.Subtle, Ellipsis: s.glyphs.Ellipsis}.Draw(frame.View)
		}})
		s.group = scene.NewGroup(scene.GroupConfig{
			Axis:    layout.Down,
			Items:   []scene.Item{{Element: s.picker.Element(), Size: layout.Flex(1)}, {Element: preview, Size: layout.Fixed(sessionPreviewRows)}, {Element: help, Size: layout.Fixed(1)}},
			Element: scene.ElementConfig{Preview: func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: s.Handle(event.Input())} }},
		})
	}
	return s.group.Element()
}

func (s *sessionCenterPane) drawPreview(view grid.View) {
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	session, ok := s.picker.Current()
	if !ok {
		view.Text(0, 1, "No loaded sessions", s.theme.Muted)
		return
	}
	detail := session.Model
	if detail != "" {
		detail += format.Separator
	}
	detail += string(session.Status) + " · updated " + compactRelativeAge(session.UpdatedAt)
	rows := [sessionPreviewRows]text.Line{
		text.Of(displayTitle(session), s.theme.Strong),
		text.Of(displayWorkspace(session.Workspace), s.theme.Context),
		text.Of(detail, s.theme.Subtle),
		text.Of(session.ID, s.theme.Muted),
	}
	for row, line := range rows {
		line.Truncate(width, s.glyphs.Ellipsis).Draw(view, 0, row)
	}
}

func (s *sessionCenterPane) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() || key.Mods != input.Alt {
		return false
	}
	s.picker.cancelPress()
	if key.Rune == 'l' {
		if s.loadMore != nil {
			s.loadMore()
		}
		return true
	}
	act, known := s.sessionShortcut(key.Rune)
	if !known {
		return false
	}
	if session, selected := s.picker.Current(); selected && act != nil {
		act(session)
	}
	return true
}

func (s *sessionCenterPane) sessionShortcut(key rune) (func(conversation.Session), bool) {
	switch key {
	case 'f':
		return s.toggleFavorite, true
	case 'r':
		return s.rename, true
	case 'd':
		return s.delete, true
	default:
		return nil, false
	}
}
