package cancelread

import (
	"context"
	"errors"
	"testing"
)

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(buffer []byte) (int, error) { return f(buffer) }

// TestReaderReportsACancellationThatLandedDuringTheRead is the half a
// before-only check misses. The read had already returned bytes, so its own
// nil error would win and the caller would keep going past the cancellation.
func TestReaderReportsACancellationThatLandedDuringTheRead(t *testing.T) {
	t.Parallel()

	cause := errors.New("read canceled")
	ctx, cancel := context.WithCancelCause(t.Context())
	source := readerFunc(func(buffer []byte) (int, error) {
		cancel(cause)
		return copy(buffer, "data"), nil
	})

	buffer := make([]byte, 8)
	read, err := Reader(ctx, source).Read(buffer)
	if read != len("data") || !errors.Is(err, cause) {
		t.Fatalf("Read = (%d, %v), want %d bytes and the cancellation cause", read, err, len("data"))
	}
}

// TestReaderRefusesBeforeTouchingAnAlreadyCanceledSource keeps the other half:
// a cancellation that already happened costs no I/O, and the reason survives
// rather than collapsing to context.Canceled.
func TestReaderRefusesBeforeTouchingAnAlreadyCanceledSource(t *testing.T) {
	t.Parallel()

	cause := errors.New("already canceled")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	touched := false
	source := readerFunc(func([]byte) (int, error) {
		touched = true
		return 0, nil
	})

	read, err := Reader(ctx, source).Read(make([]byte, 8))
	switch {
	case touched:
		t.Fatal("Reader touched a source whose context was already canceled")
	case read != 0 || !errors.Is(err, cause):
		t.Fatalf("Read = (%d, %v), want the cancellation cause", read, err)
	case errors.Is(err, context.Canceled) && !errors.Is(err, cause):
		t.Fatal("Reader collapsed the cause to context.Canceled")
	}
}
