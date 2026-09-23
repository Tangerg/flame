package capture

import (
	"io"
	"strings"
	"testing"
)

func TestWriterKeepsThePrefixAndReportsTheRest(t *testing.T) {
	writer := NewWriter(4)
	written, err := writer.Write([]byte("abc"))
	if written != 3 || err != nil || writer.Truncated() {
		t.Fatalf("first write = (%d, %v), truncated=%t", written, err, writer.Truncated())
	}
	// A producer must see its whole write accepted even when the bound is hit,
	// so a child process is never blocked by the reader that bounds it.
	written, err = writer.Write([]byte("defgh"))
	if written != 5 || err != nil || !writer.Truncated() {
		t.Fatalf("overflowing write = (%d, %v), truncated=%t", written, err, writer.Truncated())
	}
	if got := writer.String(); got != "abcd" {
		t.Fatalf("captured = %q, want the bounded prefix", got)
	}
	if got := string(writer.Bytes()); got != "abcd" {
		t.Fatalf("captured bytes = %q", got)
	}
}

func TestWriterWithoutRoomCapturesNothing(t *testing.T) {
	writer := NewWriter(0)
	if _, err := writer.Write([]byte(strings.Repeat("x", 8))); err != nil {
		t.Fatal(err)
	}
	if writer.String() != "" || !writer.Truncated() {
		t.Fatalf("captured = %q, truncated = %t", writer.String(), writer.Truncated())
	}
}

// TestWriterCannotBeBypassedByIOCopy holds the reason Write reports the whole
// length: io.Copy trusts that count, and a short one makes it report a failed
// copy for output the caller deliberately dropped.
func TestWriterCannotBeBypassedByIOCopy(t *testing.T) {
	writer := NewWriter(4)
	written, err := io.Copy(writer, strings.NewReader("oversized"))
	if err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	if written != int64(len("oversized")) {
		t.Fatalf("io.Copy wrote %d bytes, want a full drain", written)
	}
	if got := string(writer.Bytes()); got != "over" || !writer.Truncated() {
		t.Fatalf("captured = %q, truncated = %t", got, writer.Truncated())
	}
}
