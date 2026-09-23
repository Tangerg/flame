// Package capture bounds what a child process's output may cost the Runtime
// that started it.
package capture

import (
	"bytes"
)

// Writer keeps at most limit bytes of what is written to it and remembers that
// more arrived. Write never fails: a bounded reader that reported an error
// would stop the process it is draining, and a child blocked on a full pipe is
// a worse outcome than a truncated diagnostic. How truncation is announced is
// the caller's, because one caller shows the reader a marker and another hands
// the bytes to a decoder.
type Writer struct {
	buffer  bytes.Buffer
	limit   int
	dropped int
}

// NewWriter bounds capture at limit bytes. A non-positive limit captures
// nothing and reports truncation as soon as anything is written.
func NewWriter(limit int) *Writer {
	return &Writer{limit: limit}
}

func (w *Writer) Write(value []byte) (int, error) {
	written := len(value)
	remaining := max(w.limit-w.buffer.Len(), 0)
	if len(value) > remaining {
		w.dropped += len(value) - remaining
		value = value[:remaining]
	}
	_, _ = w.buffer.Write(value)
	return written, nil
}

// Truncated reports whether anything was dropped.
func (w *Writer) Truncated() bool { return w.dropped > 0 }

// Dropped counts the bytes that did not fit, for a caller that tells its reader
// how much of the output it is not seeing.
func (w *Writer) Dropped() int { return w.dropped }

// Bytes borrows the captured prefix.
func (w *Writer) Bytes() []byte { return w.buffer.Bytes() }

// String returns the captured prefix as text, without a truncation notice.
func (w *Writer) String() string { return w.buffer.String() }
