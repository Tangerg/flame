package capture

import (
	"context"
	"errors"
	"io"
)

type Reader struct {
	context context.Context
	source  io.Reader
}

func NewReader(ctx context.Context, source io.Reader) (*Reader, error) {
	if ctx == nil || source == nil {
		return nil, errors.New("capture: context and source are required")
	}
	return &Reader{context: ctx, source: source}, nil
}

// Cancellation stops the next read; it cannot interrupt an already blocking source.
func (r *Reader) Read(buffer []byte) (int, error) {
	if err := errors.Join(r.context.Err(), context.Cause(r.context)); err != nil {
		return 0, err
	}
	n, err := r.source.Read(buffer)
	if canceled := errors.Join(r.context.Err(), context.Cause(r.context)); canceled != nil {
		return n, errors.Join(err, canceled)
	}
	return n, err
}
