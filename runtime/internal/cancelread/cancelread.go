// Package cancelread reads through a context so a slow source cannot outlive
// the work that wanted it.
package cancelread

import (
	"context"
	"io"
)

// Reader returns r bounded by ctx, in the shape of io.LimitReader: the returned
// reader is the one to hand onward.
//
// The cause is checked on both sides of every Read. Before, so a cancellation
// that already happened costs no I/O; after, because the read that just
// returned may have blocked across the cancellation, and its own result would
// otherwise win and let the caller continue. context.Cause rather than
// ctx.Err keeps the reason the canceller gave, which is the error the caller
// reports.
func Reader(ctx context.Context, r io.Reader) io.Reader {
	return reader{ctx: ctx, reader: r}
}

type reader struct {
	ctx    context.Context
	reader io.Reader
}

func (c reader) Read(buffer []byte) (int, error) {
	if cause := context.Cause(c.ctx); cause != nil {
		return 0, cause
	}
	read, err := c.reader.Read(buffer)
	if cause := context.Cause(c.ctx); cause != nil {
		return read, cause
	}
	return read, err
}
