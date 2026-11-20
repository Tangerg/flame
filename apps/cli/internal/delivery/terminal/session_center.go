package terminal

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

type sessionCenterPane struct {
	group          *scene.Group
	theme          kit.Theme
	glyphs         kit.Glyphs
	picker         *picker[conversation.Session]
	items          []conversation.Session
	cursor         string
	seenCursors    map[string]struct{}
	loadMore       func()
	toggleFavorite func(conversation.Session)
	rename         func(conversation.Session)
	delete         func(conversation.Session)
}

func newSessionCenterPane(theme kit.Theme, glyphs kit.Glyphs, open func(conversation.Session)) *sessionCenterPane {
	center := &sessionCenterPane{theme: theme, glyphs: glyphs}
	center.picker = newPicker(theme, glyphs, "search loaded sessions",
		func(session conversation.Session) string {
			group := "Recent"
			if session.Favorite {
				group = glyphs.Taken + " Favorites"
			}
			return group + " · " + displayTitle(session)
		},
		func(session conversation.Session) string { return compactRelativeAge(session.UpdatedAt) },
		open,
	)
	center.Reset()
	return center
}

func (s *sessionCenterPane) Reset() {
	s.items, s.cursor = nil, ""
	s.seenCursors = map[string]struct{}{"": {}}
	s.picker.Reset()
	s.picker.SetItems(nil)
}

// sessionPagePosition is where a catalog page goes: it replaces the loaded
// catalog, or continues it from the pane's own cursor.
type sessionPagePosition uint8

const (
	firstSessionPage sessionPagePosition = iota + 1
	nextSessionPage
)

// requestCursor is the continuation a page at position must be requested from.
func (s *sessionCenterPane) requestCursor(position sessionPagePosition) string {
	if position == nextSessionPage {
		return s.cursor
	}
	return ""
}

func (s *sessionCenterPane) SetPage(page conversation.SessionPage, position sessionPagePosition) error {
	appendPage := position == nextSessionPage
	if !appendPage {
		s.seenCursors = map[string]struct{}{"": {}}
	}
	if page.NextCursor != "" {
		if _, exists := s.seenCursors[page.NextCursor]; exists {
			return fmt.Errorf("session catalog returned cyclic continuation cursor %q", page.NextCursor)
		}
	}
	next := slices.Clone(page.Items)
	if appendPage {
		seen := make(map[string]struct{}, len(s.items)+len(page.Items))
		for _, session := range s.items {
			seen[session.ID] = struct{}{}
		}
		for _, session := range page.Items {
			if _, duplicate := seen[session.ID]; duplicate {
				return fmt.Errorf("session page repeats previously loaded id %q", session.ID)
			}
			seen[session.ID] = struct{}{}
		}
		next = append(slices.Clone(s.items), page.Items...)
	}
	s.items, s.cursor = next, page.NextCursor
	if page.NextCursor != "" {
		s.seenCursors[page.NextCursor] = struct{}{}
	}
	s.picker.SetItems(s.items)
	return nil
}

// Replace refreshes a loaded Session in place. Where it belongs in the catalog
// is Runtime's ordering, which only a reloaded page can say.
func (s *sessionCenterPane) Replace(session conversation.Session) {
	index := slices.IndexFunc(s.items, func(loaded conversation.Session) bool { return loaded.ID == session.ID })
	if index < 0 {
		return
	}
	s.items[index] = session
	if selected, ok := s.picker.Current(); ok && selected.ID == session.ID {
		s.picker.Reset()
	}
	s.picker.SetItems(s.items)
}

func (s *sessionCenterPane) Remove(id string) {
	s.items = slices.DeleteFunc(s.items, func(session conversation.Session) bool { return session.ID == id })
	s.picker.SetItems(s.items)
}

func (s *sessionCenterPane) HasMore() bool { return s.cursor != "" }

func (s *sessionCenterPane) Element() scene.Element {
	if s.group == nil {
		preview := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { s.drawPreview(frame.View) }})
		help := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
			more := "all sessions loaded"
			if s.HasMore() {
				more = "alt+l load more"
			}
			kit.Label{Text: "alt+f favorite · alt+r rename · alt+d delete · " + more, Style: s.theme.Subtle, Ellipsis: s.glyphs.Ellipsis}.Draw(frame.View)
		}})
		s.group = scene.NewGroup(scene.GroupConfig{
			Axis:    layout.Down,
			Items:   []scene.Item{{Element: s.picker.Element(), Size: layout.Flex(1)}, {Element: preview, Size: layout.Fixed(4)}, {Element: help, Size: layout.Fixed(1)}},
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
	text.Of(displayTitle(session), s.theme.Strong).Truncate(width, s.glyphs.Ellipsis).Draw(view, 0, 0)
	text.Of(displayWorkspace(session.Workspace), s.theme.Context).Truncate(width, s.glyphs.Ellipsis).Draw(view, 0, 1)
	detail := session.Model
	if detail != "" {
		detail += " · "
	}
	detail += string(session.Status) + " · updated " + compactRelativeAge(session.UpdatedAt)
	text.Of(detail, s.theme.Subtle).Truncate(width, s.glyphs.Ellipsis).Draw(view, 0, 2)
	text.Of(session.ID, s.theme.Muted).Truncate(width, s.glyphs.Ellipsis).Draw(view, 0, 3)
}

func (s *sessionCenterPane) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() || key.Mods != input.Alt {
		return false
	}
	s.picker.cancelPress()
	if key.Rune == 'l' {
		if s.HasMore() && s.loadMore != nil {
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

// sessionShortcut is the Alt-key action on the selected session.
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
