package sandbox

import (
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/capture"
)

type repeatingReader struct{}

func (repeatingReader) Read(data []byte) (int, error) {
	for index := range data {
		data[index] = 'x'
	}
	return len(data), nil
}

// TestOutputWithMarkerReportsWhatTheModelCannotSee covers the sandbox's half of
// bounded capture: the writer counts the dropped bytes, and this is where a
// command's output says how many of them the reader is missing.
func TestOutputWithMarkerReportsWhatTheModelCannotSee(t *testing.T) {
	const produced = 2 * maxCommandOutputBytes
	output := capture.NewWriter(maxCommandOutputBytes)
	written, err := io.Copy(output, io.LimitReader(repeatingReader{}, produced))
	if err != nil {
		t.Fatal(err)
	}
	if written != produced {
		t.Fatalf("io.Copy wrote %d bytes; want %d", written, produced)
	}
	marked := string(outputWithMarker(output))
	if len(marked) <= maxCommandOutputBytes || !strings.Contains(marked, "bytes truncated") {
		t.Fatalf("marked output is %d bytes and reads %q", len(marked), marked[max(0, len(marked)-64):])
	}

	within := capture.NewWriter(maxCommandOutputBytes)
	if _, err := within.Write([]byte("complete")); err != nil {
		t.Fatal(err)
	}
	if got := string(outputWithMarker(within)); got != "complete" {
		t.Fatalf("untruncated output = %q, want no marker", got)
	}
}
