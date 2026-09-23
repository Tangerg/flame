package sideload

import (
	"io"
	"testing"
)

type endlessReader struct{}

func (endlessReader) Read(data []byte) (int, error) {
	for index := range data {
		data[index] = 'x'
	}
	return len(data), nil
}

// TestCappedBufferCannotBeBypassedByReaderFrom holds the reason the buffer is a
// field: os/exec copies a non-file Stdout with io.Copy, which hands the whole
// stream to io.ReaderFrom when the destination has one. A plugin would then
// spend the CLI's memory instead of hitting its output limit.
func TestCappedBufferCannotBeBypassedByReaderFrom(t *testing.T) {
	output := &cappedBuffer{limit: maxCommandOutputBytes}
	if _, bypasses := any(output).(io.ReaderFrom); bypasses {
		t.Fatal("capped buffer exposes io.ReaderFrom and can bypass its limit")
	}
	const produced = 4 * maxCommandOutputBytes
	written, err := io.Copy(output, io.LimitReader(endlessReader{}, produced))
	if err != nil {
		t.Fatal(err)
	}
	if written != produced {
		t.Fatalf("io.Copy wrote %d bytes, want the producer's full drain of %d", written, produced)
	}
	if len(output.Bytes()) != maxCommandOutputBytes || !output.overflow {
		t.Fatalf("captured %d bytes, overflow=%t", len(output.Bytes()), output.overflow)
	}
}
