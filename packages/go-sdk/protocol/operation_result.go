package protocol

import (
	"fmt"
	"iter"
	"reflect"
)

type OperationResult struct {
	Value   any
	Events  iter.Seq2[any, error]
	Failure *Failure
}

func (r OperationResult) ValueAs[T any]() (T, bool) {
	var zero T
	if r.Value == nil && reflect.TypeFor[T]().Kind() == reflect.Interface {
		return zero, true
	}
	value, ok := r.Value.(T)
	return value, ok
}

func (r OperationResult) Response[T any](method string) (T, error) {
	var zero T
	if r.Failure != nil {
		return zero, r.Failure
	}
	value, ok := r.ValueAs[T]()
	if !ok {
		return zero, fmt.Errorf("%s returned an invalid response type: %w", method, ErrInvalidResponse)
	}
	return value, nil
}

func (r OperationResult) Stream[Ack, Event any](method string) (Ack, iter.Seq2[Event, error], error) {
	var zero Ack
	if r.Failure != nil {
		return zero, nil, r.Failure
	}
	ack, ok := r.ValueAs[Ack]()
	if !ok || r.Events == nil {
		return zero, nil, fmt.Errorf("%s returned an invalid stream: %w", method, ErrInvalidResponse)
	}
	return ack, func(yield func(Event, error) bool) {
		var zero Event
		for value, err := range r.Events {
			if err != nil {
				yield(zero, err)
				return
			}
			event, ok := value.(Event)
			if !ok {
				yield(zero, fmt.Errorf("%s returned an invalid event type: %w", method, ErrInvalidResponse))
				return
			}
			if !yield(event, nil) {
				return
			}
		}
	}, nil
}
