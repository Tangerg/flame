package terminal

import (
	"context"
	"sync"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/text"
)

type readingSearchResult struct {
	base.Result
	Err     error
	request *readingSearchRequest
}

type readingSearchRequest struct {
	config  base.ScanConfig
	context context.Context
	cancel  context.CancelFunc
}

// One worker owns search work for a reading session. Replacement cancels the old
// immutable row scan, and its exact request identity gates delivery even when the
// query text repeats. Results and pending work each retain at most one snapshot.
type readingSearch struct {
	mu       sync.Mutex
	requests chan *readingSearchRequest
	results  chan readingSearchResult
	stop     chan struct{}
	done     chan struct{}
	current  *readingSearchRequest
	once     sync.Once
}

func newReadingSearch() *readingSearch {
	search := &readingSearch{requests: make(chan *readingSearchRequest, 1), results: make(chan readingSearchResult, 1), stop: make(chan struct{}), done: make(chan struct{})}
	go search.run()
	return search
}

func (s *readingSearch) Submit(rows []text.Row, query string, start int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.stop:
		return
	default:
	}
	if s.current != nil {
		s.current.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	request := &readingSearchRequest{config: base.ScanConfig{Query: query, Rows: rows, StartRow: start}, context: ctx, cancel: cancel}
	s.current = request
	select {
	case <-s.requests:
	default:
	}
	select {
	case <-s.results:
	default:
	}
	select {
	case s.requests <- request:
	case <-s.stop:
		cancel()
	}
}

func (s *readingSearch) Current(result readingSearchResult) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return result.request != nil && s.current == result.request && result.request.context.Err() == nil
}
func (s *readingSearch) Results() <-chan readingSearchResult { return s.results }
func (s *readingSearch) Close() {
	s.once.Do(func() {
		s.mu.Lock()
		if s.current != nil {
			s.current.cancel()
		}
		close(s.stop)
		s.mu.Unlock()
		<-s.done
	})
}
func (s *readingSearch) run() {
	defer close(s.done)
	defer close(s.results)
	for {
		select {
		case <-s.stop:
			return
		case request := <-s.requests:
			result, err := base.Scan(request.context, request.config)
			s.publish(readingSearchResult{Result: result, Err: err, request: request})
		}
	}
}

func (s *readingSearch) publish(result readingSearchResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != result.request || result.request.context.Err() != nil {
		return
	}
	select {
	case <-s.results:
	default:
	}
	s.results <- result
}
