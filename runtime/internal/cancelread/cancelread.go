// Package cancelread observes cancellation across reads without hiding I/O failures.
package cancelread

import (
	"context"
	"errors"
	"io"
)

// Reader checks cancellation before and after each read; it cannot interrupt
// a read blocked inside r. Errors retain the standard cancellation category,
// its custom cause, and any concurrent I/O failure.
func Reader(ctx context.Context, r io.Reader) io.Reader {
	return reader{ctx: ctx, reader: r}
}

type reader struct {
	ctx    context.Context
	reader io.Reader
}

func (c reader) Read(buffer []byte) (int, error) {
	if err := c.cancellation(); err != nil {
		return 0, err
	}
	read, err := c.reader.Read(buffer)
	if cancelErr := c.cancellation(); cancelErr != nil {
		err = errors.Join(err, cancelErr)
	}
	return read, err
}

func (c reader) cancellation() error {
	err := c.ctx.Err()
	if err == nil {
		return nil
	}
	cause := context.Cause(c.ctx)
	if errors.Is(cause, err) {
		return cause
	}
	return errors.Join(err, cause)
}
