package pluginservices

import (
	"errors"

	"github.com/Tangerg/scope/core/chat"
)

// modelStreamSender retains the terminal outcome until deferred settlement and
// release have joined. Chunks remain synchronous; a stopped or panicking
// consumer cannot receive another yield from cleanup.
type modelStreamSender struct {
	yield    func(*chat.ResponseDelta, error) bool
	stopped  bool
	terminal *chat.ResponseDelta
	failure  error
}

func newModelStreamSender(yield func(*chat.ResponseDelta, error) bool) *modelStreamSender {
	return &modelStreamSender{yield: yield}
}

func (s *modelStreamSender) send(value *chat.ResponseDelta, err error) bool {
	if s.stopped || s.failure != nil {
		return false
	}
	if err != nil {
		if value != nil {
			// The provider can reuse its delta after its yield returns.
			s.terminal = value.Clone()
		}
		s.fail(err)
		return false
	}
	s.stopped = true
	s.stopped = !s.yield(value, nil)
	return !s.stopped
}

func (s *modelStreamSender) fail(err error) {
	s.failure = errors.Join(s.failure, err)
}

func (s *modelStreamSender) finish() {
	if s.stopped {
		return
	}
	s.stopped = true
	if s.failure != nil {
		s.yield(s.terminal, s.failure)
	}
}
