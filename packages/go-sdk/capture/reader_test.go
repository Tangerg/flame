package capture

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReaderKeepsCancellationAndSourceOutcome(t *testing.T) {
	cause := errors.New("caller stopped")
	sourceFailure := errors.New("source failed")
	ctx, cancel := context.WithCancelCause(t.Context())
	reader, err := NewReader(ctx, readFunc(func(buffer []byte) (int, error) {
		cancel(cause)
		return copy(buffer, "data"), sourceFailure
	}))
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4)
	n, err := reader.Read(buffer)
	if string(buffer[:n]) != "data" || !errors.Is(err, cause) || !errors.Is(err, context.Canceled) || !errors.Is(err, sourceFailure) {
		t.Fatalf("read = %q, %v", buffer[:n], err)
	}
}

func TestReaderDoesNotReadAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	reader, err := NewReader(ctx, readFunc(func([]byte) (int, error) {
		t.Fatal("read after cancellation")
		return 0, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Read(make([]byte, 1)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestReaderCapturesUncancelledSource(t *testing.T) {
	reader, err := NewReader(t.Context(), strings.NewReader("complete"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(reader)
	if err != nil || string(got) != "complete" {
		t.Fatalf("capture = %q, %v", got, err)
	}
	if _, err := NewReader(nil, strings.NewReader("")); err == nil {
		t.Fatal("accepted missing context")
	}
	if _, err := NewReader(t.Context(), nil); err == nil {
		t.Fatal("accepted missing source")
	}
}

type readFunc func([]byte) (int, error)

func (f readFunc) Read(buffer []byte) (int, error) { return f(buffer) }
