package terminal

import (
	"context"
	"fmt"
	"github.com/Tangerg/oolong/ui/core/text"
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

type transcriptSearch struct {
	worker   *readingSearch
	query    string
	announce bool
	matches  []base.Match
	current  int
	cursor   transcriptSearchCursor
}

type transcriptSearchCursor struct {
	blockID           transcriptEntryID
	rowOffset, column int
	index             int
	present           bool
}

func newTranscriptSearch() transcriptSearch {
	return transcriptSearch{worker: newReadingSearch(), current: -1}
}

func (s *transcriptSearch) Close() {
	if s.worker != nil {
		s.worker.Close()
	}
}

func (s *transcriptSearch) presentation() ([]base.Match, int) {
	return s.matches, s.current
}

func (s *transcriptSearch) Find(view *transcriptView, query string) {
	s.query = strings.TrimSpace(query)
	s.announce = s.query != ""
	s.matches, s.current = nil, -1
	s.cursor = transcriptSearchCursor{}
	s.worker.Submit(view.searchRows(), s.query, 0)
}

func (s *transcriptSearch) Refresh(view *transcriptView) {
	if s.query != "" {
		s.worker.Submit(view.searchRows(), s.query, 0)
	}
}

func (s *transcriptSearch) Reset() {
	s.query, s.matches, s.current, s.announce = "", nil, -1, false
	s.cursor = transcriptSearchCursor{}
	s.worker.Submit(nil, "", 0)
}

func (s *transcriptSearch) Results() <-chan readingSearchResult { return s.worker.Results() }

func (s *transcriptSearch) Accept(view *transcriptView, result readingSearchResult) (accepted, announce bool) {
	if !s.worker.Current(result) {
		return false, false
	}
	next := s.matchIndex(view, result.Matches)
	s.matches = result.Matches
	if len(s.matches) > 0 {
		s.current = next
		s.rememberCursor(view)
	} else {
		s.current = -1
		s.cursor = transcriptSearchCursor{}
	}
	announce, s.announce = s.announce, false
	return true, announce
}

func (s *transcriptSearch) Step(view *transcriptView, delta int) bool {
	if len(s.matches) == 0 {
		return false
	}
	s.current = (s.current + delta) % len(s.matches)
	if s.current < 0 {
		s.current += len(s.matches)
	}
	s.rememberCursor(view)
	return true
}

func (s *transcriptSearch) matchIndex(view *transcriptView, matches []base.Match) int {
	if len(matches) == 0 || !s.cursor.present {
		return 0
	}
	best := -1
	var bestRowDistance, bestColumnDistance uint
	for index, match := range matches {
		id, offset, ok := view.entryAtRow(match.Row)
		if !ok || id != s.cursor.blockID {
			continue
		}
		column := 0
		if len(match.Spans) > 0 {
			column = match.Spans[0].Col
		}
		rowDistance := unsignedDistance(offset, s.cursor.rowOffset)
		columnDistance := unsignedDistance(column, s.cursor.column)
		if best < 0 || rowDistance < bestRowDistance ||
			(rowDistance == bestRowDistance && columnDistance < bestColumnDistance) {
			best, bestRowDistance, bestColumnDistance = index, rowDistance, columnDistance
		}
	}
	if best >= 0 {
		return best
	}
	return min(s.cursor.index, len(matches)-1)
}

func (s *transcriptSearch) rememberCursor(view *transcriptView) {
	if s.current < 0 || s.current >= len(s.matches) {
		s.cursor = transcriptSearchCursor{}
		return
	}
	match := s.matches[s.current]
	id, offset, ok := view.entryAtRow(match.Row)
	if !ok {
		s.cursor = transcriptSearchCursor{}
		return
	}
	column := 0
	if len(match.Spans) > 0 {
		column = match.Spans[0].Col
	}
	s.cursor = transcriptSearchCursor{
		blockID: id, rowOffset: offset, column: column, index: s.current, present: true,
	}
}

func unsignedDistance(left, right int) uint {
	if left < right {
		left, right = right, left
	}
	return uint(left) - uint(right)
}

func (t *transcriptView) Find(query string) { t.search.Find(t, query) }

func (t *transcriptView) refreshSearch() { t.search.Refresh(t) }

func (t *transcriptView) SearchResults() <-chan readingSearchResult { return t.search.Results() }

func (t *transcriptView) AcceptSearch(result readingSearchResult) (accepted, announce bool) {
	accepted, announce = t.search.Accept(t, result)
	if accepted && announce {
		t.revealSearchMatch()
	}
	return accepted, announce
}

func (t *transcriptView) StepMatch(delta int) bool {
	if !t.search.Step(t, delta) {
		return false
	}
	return t.revealSearchMatch()
}

func (t *transcriptView) revealSearchMatch() bool {
	if t.search.current < 0 || t.search.current >= len(t.search.matches) {
		return false
	}
	match := t.search.matches[t.search.current]
	t.scroll.Reveal(match.Row, match.Row+max(len(match.Spans)-1, 0))
	return true
}

func (s *transcriptSearch) presentationFor(layout base.TranscriptLayout) ([]base.Match, int) {
	if s.query == "" {
		return nil, -1
	}
	result, err := base.Scan(context.Background(), base.ScanConfig{Query: s.query, Rows: layout.Rows(layout.StartRow(), layout.Height()), StartRow: layout.StartRow()})
	if err != nil {
		return nil, -1
	}
	return result.Matches, min(s.current, len(result.Matches)-1)
}

func (t *transcriptView) searchRows() []text.Row {
	width := max(t.projected.Value().Width(), 1)
	var rows []text.Row
	for _, id := range t.order {
		if body := t.entries[id].passive(); body != nil {
			rows = append(rows, body.Rows(width)...)
		}
	}
	return rows
}

func (t *transcriptView) entryAtRow(row int) (transcriptEntryID, int, bool) {
	width, top := max(t.projected.Value().Width(), 1), 0
	for _, id := range t.order {
		body := t.entries[id].passive()
		if body == nil {
			continue
		}
		height := body.HeightForWidth(width)
		if row >= top && row < top+height {
			return id, row - top, true
		}
		top += height
	}
	return 0, 0, false
}

// liveSearch finds text in the live transcript: it asks for the query,
// follows the transcript's background search results and steps between
// matches. transcript is the transcript presented now.
type liveSearch struct {
	dialog     *presentationDialog
	field      *base.Text
	transcript func() *transcriptView
	tasks      taskRunner
	notify     func(string)
}

func newLiveSearch(stack *workspaceScene, theme kit.Theme, glyphs kit.Glyphs, tasks taskRunner, transcript func() *transcriptView, notify func(string)) *liveSearch {
	p := &liveSearch{transcript: transcript, tasks: tasks, notify: notify}
	field := base.NewText(base.TextConfig{Label: "Find in the live transcript", Placeholder: "text", Initial: "", Check: requiredText})
	p.field = field
	form := kit.FormConfig{Fields: []base.Field{field}, Theme: theme, Glyphs: glyphs,
		Hints: []keymap.Action{base.Submit, base.Cancel}}
	form.Keys = base.DefaultFormKeys()
	form.Submit = func(base.SubmissionID) (scene.ActionState, error) {
		if !p.dialog.Open() {
			return scene.Unchanged, nil
		}
		p.dialog.Dismiss()
		p.find(field.Text())
		return scene.Applied, nil
	}

	dressed := kit.NewForm(form)
	p.dialog = newPresentationDialog(stack, kit.DialogConfig{Theme: theme, Glyphs: glyphs, Title: "Search", Body: formNavigation(dressed.Element(), func() { p.dialog.Dismiss() }),
		Where: layout.Placement{Width: 68, Height: 7},
	})
	return p
}

func (p *liveSearch) show() {
	p.field.SetText("")
	p.dialog.Show()
}

func (p *liveSearch) find(query string) {
	p.transcript().Find(query)
	p.notify("searching for " + query)
}

func (p *liveSearch) step(delta int) {
	if !p.transcript().StepMatch(delta) {
		p.notify("no active search matches")
	}
}

// listen follows the search results of the presented transcript, replacing
// the subscription to a transcript no longer presented.
func (p *liveSearch) listen() {
	results := p.transcript().SearchResults()
	operations := p.tasks.operations
	operations.Go(searchOperation, true, func(ctx context.Context, lease operationLease) {
		for {
			select {
			case result, ok := <-results:
				if !ok {
					return
				}
				if err := post(ctx, p.tasks.dispatch, func() {
					if operations.Current(lease) {
						p.accept(result)
					}
				}); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	})
}

func (p *liveSearch) accept(result readingSearchResult) {
	if !p.transcript().search.worker.Current(result) {
		return
	}
	if result.Err != nil {
		p.notify(fmt.Sprintf("search failed: %v", result.Err))
		return
	}
	accepted, announce := p.transcript().AcceptSearch(result)
	if accepted && announce {
		p.notify(fmt.Sprintf("%d match(es) for %q", len(result.Matches), result.Query))
	}
}
