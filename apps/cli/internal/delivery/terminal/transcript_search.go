package terminal

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

type transcriptSearch struct {
	worker   *headless.Search
	query    string
	announce bool
	matches  []headless.Match
	current  int
	cursor   transcriptSearchCursor
}

type transcriptSearchCursor struct {
	blockID           headless.BlockID
	rowOffset, column int
	index             int
	present           bool
}

func newTranscriptSearch() transcriptSearch {
	return transcriptSearch{worker: headless.NewSearch(), current: -1}
}

func (s *transcriptSearch) Close() {
	if s.worker != nil {
		s.worker.Close()
	}
}

func (s *transcriptSearch) presentation() ([]headless.Match, int) {
	return s.matches, s.current
}

func (s *transcriptSearch) Find(content *headless.Transcript, query string) {
	s.query = strings.TrimSpace(query)
	s.announce = s.query != ""
	s.matches, s.current = nil, -1
	s.cursor = transcriptSearchCursor{}
	s.worker.Submit(content, s.query, false)
}

func (s *transcriptSearch) Refresh(content *headless.Transcript) {
	if s.query != "" {
		s.worker.Submit(content, s.query, false)
	}
}

func (s *transcriptSearch) Reset(content *headless.Transcript) {
	s.query, s.matches, s.current, s.announce = "", nil, -1, false
	s.cursor = transcriptSearchCursor{}
	s.worker.Submit(content, "", false)
}

func (s *transcriptSearch) Results() <-chan headless.Result { return s.worker.Results() }

func (s *transcriptSearch) Accept(content *headless.Transcript, result headless.Result) (accepted, announce bool) {
	if !result.Current() {
		return false, false
	}
	next := s.matchIndex(content, result.Matches)
	s.matches = result.Matches
	if len(s.matches) > 0 {
		s.current = next
		s.rememberCursor(content)
	} else {
		s.current = -1
		s.cursor = transcriptSearchCursor{}
	}
	announce, s.announce = s.announce, false
	return true, announce
}

func (s *transcriptSearch) Step(content *headless.Transcript, delta int) bool {
	if len(s.matches) == 0 {
		return false
	}
	s.current = (s.current + delta) % len(s.matches)
	if s.current < 0 {
		s.current += len(s.matches)
	}
	s.rememberCursor(content)
	return true
}

func (s *transcriptSearch) matchIndex(content *headless.Transcript, matches []headless.Match) int {
	if len(matches) == 0 || !s.cursor.present {
		return 0
	}
	best := -1
	var bestRowDistance, bestColumnDistance uint
	for index, match := range matches {
		id, offset, ok := content.At(match.Row)
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

func (s *transcriptSearch) rememberCursor(content *headless.Transcript) {
	if s.current < 0 || s.current >= len(s.matches) {
		s.cursor = transcriptSearchCursor{}
		return
	}
	match := s.matches[s.current]
	id, offset, ok := content.At(match.Row)
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

func (t *transcriptView) Find(query string) { t.search.Find(&t.content, query) }

func (t *transcriptView) refreshSearch() { t.search.Refresh(&t.content) }

func (t *transcriptView) SearchResults() <-chan headless.Result { return t.search.Results() }

func (t *transcriptView) AcceptSearch(result headless.Result) (accepted, announce bool) {
	accepted, announce = t.search.Accept(&t.content, result)
	if accepted && announce {
		t.view.Matches, t.view.Current = t.search.presentation()
		t.view.RevealMatch(t.view.Current)
	}
	return accepted, announce
}

func (t *transcriptView) StepMatch(delta int) bool {
	if !t.search.Step(&t.content, delta) {
		return false
	}
	t.view.Matches, t.view.Current = t.search.presentation()
	return t.view.RevealMatch(t.view.Current)
}

// liveSearch finds text in the live transcript: it asks for the query,
// follows the transcript's background search results and steps between
// matches. transcript is the transcript presented now.
type liveSearch struct {
	dialog     *presentationDialog
	query      string
	transcript func() *transcriptView
	tasks      taskRunner
	notify     func(string)
}

func newLiveSearch(stack *headless.Stack, theme kit.Theme, glyphs kit.Glyphs, tasks taskRunner, transcript func() *transcriptView, notify func(string)) *liveSearch {
	p := &liveSearch{transcript: transcript, tasks: tasks, notify: notify}
	field := headless.NewText(headless.TextConfig{Label: "Find in the live transcript", Placeholder: "text", Value: headless.Bind(&p.query), Check: requiredText})
	form := headless.NewForm(field)
	form.Keys = headless.DefaultFormKeys()
	form.Done = func() {
		if !p.dialog.Open() {
			return
		}
		p.dialog.Dismiss()
		p.find(p.query)
	}
	form.GaveUp = func() { p.dialog.Dismiss() }
	dressed := kit.NewForm(kit.FormConfig{
		Theme: theme, Glyphs: glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	p.dialog = newPresentationDialog(kit.DialogConfig{
		Stack: stack, Theme: theme, Glyphs: glyphs, Title: "Search", Body: dressed,
		Where: layout.Placement{Width: 68, Height: 7},
	})
	return p
}

func (p *liveSearch) show() {
	p.query = ""
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

func (p *liveSearch) accept(result headless.Result) {
	if result.Err != nil {
		p.notify(fmt.Sprintf("search failed: %v", result.Err))
		return
	}
	accepted, announce := p.transcript().AcceptSearch(result)
	if accepted && announce {
		p.notify(fmt.Sprintf("%d match(es) for %q", len(result.Matches), result.Query))
	}
}
