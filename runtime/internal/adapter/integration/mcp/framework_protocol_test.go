package mcp

import (
	"errors"
	"io"
	"testing"

	"github.com/Tangerg/frame"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestFrameWriterPreservesTransportRefusalsAndOtherIOFailures(t *testing.T) {
	t.Run("Frame channel refusal", func(t *testing.T) {
		reader, _ := io.Pipe()
		defer reader.Close()
		writer := frameWriter{closedFrameChannel{reader}}
		n, err := writer.Write([]byte("request"))
		if n != 0 || !errors.Is(err, sdk.ErrConnectionClosed) || !errors.Is(err, frame.ErrClosed) {
			t.Fatalf("closed Frame write = %d, %v", n, err)
		}
	})
	t.Run("other I/O failure", func(t *testing.T) {
		reader, pipe := io.Pipe()
		defer pipe.Close()
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
		n, err := (frameWriter{pipe}).Write([]byte("request"))
		if n != 0 || !errors.Is(err, io.ErrClosedPipe) || errors.Is(err, sdk.ErrConnectionClosed) {
			t.Fatalf("unrelated write failure = %d, %v", n, err)
		}
	})
}
