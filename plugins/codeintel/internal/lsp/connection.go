package lsp

import (
	"context"
	"errors"
	"sync"

	"github.com/sourcegraph/jsonrpc2"
)

type connection struct {
	*jsonrpc2.Conn
	stream *connectionStream
}

type connectionStream struct {
	jsonrpc2.ObjectStream
	close func() error
}

func newConnection(ctx context.Context, stream jsonrpc2.ObjectStream, handler jsonrpc2.Handler) *connection {
	owned := &connectionStream{ObjectStream: stream, close: sync.OnceValue(stream.Close)}
	return &connection{
		Conn:   jsonrpc2.NewConn(ctx, owned, handler),
		stream: owned,
	}
}

func (s *connectionStream) Close() error { return s.close() }

// jsonrpc2 marks logical closure before releasing its stream. The stream owns
// physical completion and its result even when another closer got there first.
func (c *connection) Close() error {
	err := c.Conn.Close()
	streamErr := c.stream.Close()
	if errors.Is(err, jsonrpc2.ErrClosed) {
		return streamErr
	}
	return err
}
