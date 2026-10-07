package terminal

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

type sessionCenterPane struct {
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

func (s *sessionCenterPane) SetPage(page conversation.SessionPage, appendPage bool) error {
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

func (s *sessionCenterPane) Cursor() string { return s.cursor }

func (s *sessionCenterPane) Draw(frame headless.Frame) {
	rows := frame.Subs((layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(4)},
		{Size: layout.Fixed(1)},
	}))
	s.picker.Draw(rows[0])
	s.drawPreview(rows[1].View)
	more := "all sessions loaded"
	if s.HasMore() {
		more = "alt+l load more"
	}
	help := "alt+f favorite · alt+r rename · alt+d delete · " + more
	kit.Label{Text: help, Style: s.theme.Subtle, Ellipsis: s.glyphs.Ellipsis}.Draw(rows[2].View)
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
	view.Text(0, 0, text.Truncate(displayTitle(session), width, s.glyphs.Ellipsis), s.theme.Strong)
	view.Text(0, 1, text.Truncate(displayWorkspace(session.Workspace), width, s.glyphs.Ellipsis), s.theme.Context)
	detail := session.Model
	if detail != "" {
		detail += " · "
	}
	detail += string(session.Status) + " · updated " + compactRelativeAge(session.UpdatedAt)
	view.Text(0, 2, text.Truncate(detail, width, s.glyphs.Ellipsis), s.theme.Subtle)
	view.Text(0, 3, text.Truncate(session.ID, width, s.glyphs.Ellipsis), s.theme.Muted)
}

func (s *sessionCenterPane) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() && key.Mods == input.Alt {
		s.picker.interruptPointerGesture()
		session, selected := s.picker.Current()
		switch key.Rune {
		case 'l':
			if s.HasMore() && s.loadMore != nil {
				s.loadMore()
			}
			return true
		case 'f':
			if selected && s.toggleFavorite != nil {
				s.toggleFavorite(session)
			}
			return true
		case 'r':
			if selected && s.rename != nil {
				s.rename(session)
			}
			return true
		case 'd':
			if selected && s.delete != nil {
				s.delete(session)
			}
			return true
		}
	}
	return s.picker.Handle(event)
}

func (s *sessionCenterPane) Focus(has bool) { s.picker.Focus(has) }
