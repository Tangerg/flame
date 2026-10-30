package pluginservices

// streamSender is the one place a stream iteration delivers: once the consumer
// refuses an item, nothing else is sent, including a deferred cleanup failure.
// Calling yield after it returned false violates the range-over-func contract.
type streamSender[T any] struct {
	yield   func(T, error) bool
	stopped bool
}

func newStreamSender[T any](yield func(T, error) bool) *streamSender[T] {
	return &streamSender[T]{yield: yield}
}

func (s *streamSender[T]) send(value T, err error) bool {
	if s.stopped {
		return false
	}
	s.stopped = !s.yield(value, err)
	return !s.stopped
}

func (s *streamSender[T]) fail(err error) {
	var zero T
	s.send(zero, err)
}
