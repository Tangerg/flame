package terminal

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/program"
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
	if result.Query != s.query {
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
	return t.search.Accept(&t.content, result)
}

func (t *transcriptView) StepMatch(delta int) bool { return t.search.Step(&t.content, delta) }

func (a *app) listenForSearch() {
	results := a.transcript.SearchResults()
	dispatcher := a.loop.Dispatcher()
	a.operations.Go(searchOperation, true, func(ctx context.Context, lease operationLease) {
		for {
			select {
			case result, ok := <-results:
				if !ok {
					return
				}
				if err := a.postSearchResult(ctx, dispatcher, lease, result); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	})
}

func (a *app) postSearchResult(ctx context.Context, dispatcher program.Dispatcher, lease operationLease, result headless.Result) error {
	return post(ctx, dispatcher, func() {
		if !a.operations.Current(lease) || a.closed {
			return
		}
		a.acceptSearchResult(result)
	})
}

func (a *app) acceptSearchResult(result headless.Result) {
	if result.Err != nil {
		a.message(fmt.Sprintf("search failed: %v", result.Err))
		return
	}
	accepted, announce := a.transcript.AcceptSearch(result)
	if accepted && announce {
		a.message(fmt.Sprintf("%d match(es) for %q", len(result.Matches), result.Query))
	}
}
