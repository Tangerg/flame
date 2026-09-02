package promptsource

import (
	"context"
	"errors"
	"testing"
)

func TestReadAuthoredPromptFilePreservesCancellationCause(t *testing.T) {
	cause := errors.New("authored prompt canceled")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)

	if _, err := readAuthoredPromptFile(ctx, "unused"); !errors.Is(err, cause) {
		t.Fatalf("readAuthoredPromptFile error = %v, want cancellation cause", err)
	}
}

func TestPromptContextReaderReportsCancellationAfterRead(t *testing.T) {
	cause := errors.New("read canceled")
	ctx, cancel := context.WithCancelCause(t.Context())
	reader := promptContextReader{
		ctx: ctx,
		reader: readerFunc(func(buffer []byte) (int, error) {
			cancel(cause)
			return copy(buffer, "data"), nil
		}),
	}

	buffer := make([]byte, 8)
	read, err := reader.Read(buffer)
	if read != len("data") || !errors.Is(err, cause) {
		t.Fatalf("Read = %d, %v; want %d bytes and cancellation cause", read, err, len("data"))
	}
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(buffer []byte) (int, error) {
	return f(buffer)
}
