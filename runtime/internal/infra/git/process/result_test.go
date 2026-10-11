package process

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(buffer []byte) (int, error) { return f(buffer) }

func TestOutputAdmissionRetainsCapacityCancellationAndReadFailure(t *testing.T) {
	cause := errors.New("observation retired")
	failure := errors.New("pipe failed")
	ctx, cancel := context.WithCancelCause(t.Context())
	remaining := maxOutputBytes + 1
	source := readerFunc(func(buffer []byte) (int, error) {
		n := min(remaining, len(buffer))
		clear(buffer[:n])
		remaining -= n
		if remaining == 0 {
			cancel(cause)
			return n, failure
		}
		return n, nil
	})
	_, err := readOutput(ctx, source)
	for _, want := range []error{ErrOutputTooLarge, context.Canceled, cause, failure} {
		if !errors.Is(err, want) {
			t.Fatalf("output admission lost %v: %v", want, err)
		}
	}
}

func TestOutputAdmissionKeepsOrdinaryEOF(t *testing.T) {
	source := readerFunc(func(buffer []byte) (int, error) { return copy(buffer, "complete"), io.EOF })
	output, err := readOutput(t.Context(), source)
	if err != nil || string(output) != "complete" {
		t.Fatalf("output admission = %q, %v; want complete output", output, err)
	}
}

func TestRunPreservesCancellationCategoryAndCauseBeforeDispatch(t *testing.T) {
	cause := errors.New("workspace retired")
	canceled, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	expired, endDeadline := context.WithDeadlineCause(t.Context(), time.Unix(0, 0), cause)
	defer endDeadline()
	for _, test := range []struct {
		name     string
		ctx      context.Context
		category error
	}{
		{"canceled", canceled, context.Canceled},
		{"deadline", expired, context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := Run(test.ctx, nil, "--version")
			if !errors.Is(err, test.category) || !errors.Is(err, cause) {
				t.Fatalf("Run = %+v, %v; want cancellation category and owner cause", result, err)
			}
		})
	}
}
