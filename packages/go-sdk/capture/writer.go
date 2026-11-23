package capture

import (
	"bytes"
	"strconv"
	"unicode/utf8"
)

// Write never fails: a bounded reader that reported an error would stop the
// process it is draining, and a child blocked on a full pipe is worse than a
// truncated diagnostic.
type Writer struct {
	buffer  bytes.Buffer
	limit   int
	dropped int
}

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

func (w *Writer) Truncated() bool { return w.dropped > 0 }

func (w *Writer) Dropped() int { return w.dropped }

func (w *Writer) Bytes() []byte { return w.buffer.Bytes() }

func (w *Writer) String() string { return w.buffer.String() }

func (w *Writer) Diagnostic() string {
	value := w.String()
	if utf8.ValidString(value) {
		return value
	}
	escaped := strconv.QuoteToASCII(value)
	return escaped[:min(len(escaped), w.limit)]
}
