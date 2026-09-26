package runtime

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/Tangerg/sse"
	"golang.org/x/text/encoding"
)

// A call remains owned until its body is consumed, its context ends, or its
// Client closes. A returned but unconsumed iterator must not retain a socket.
type remoteCall struct {
	client   *Client
	ctx      context.Context
	cancel   context.CancelCauseFunc
	mu       sync.Mutex
	body     io.ReadCloser
	stop     func() bool
	finished bool
}

func (c *Client) begin(parent context.Context) (*remoteCall, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	ctx, cancel := context.WithCancelCause(parent)
	call := &remoteCall{client: c, ctx: ctx, cancel: cancel}
	c.calls[call] = struct{}{}
	c.mu.Unlock()
	stop := context.AfterFunc(ctx, func() { call.finish(context.Cause(ctx)) })
	call.mu.Lock()
	if call.finished {
		stop()
	} else {
		call.stop = stop
	}
	call.mu.Unlock()
	return call, nil
}

func (c *remoteCall) attach(body io.ReadCloser) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finished {
		_ = body.Close()
		return context.Cause(c.ctx)
	}
	c.body = body
	return nil
}

func (c *remoteCall) finish(cause error) {
	c.mu.Lock()
	if c.finished {
		c.mu.Unlock()
		return
	}
	c.finished = true
	body, stop := c.body, c.stop
	c.cancel(cause)
	c.mu.Unlock()
	if stop != nil {
		stop()
	}
	if body != nil {
		_ = body.Close()
	}
	c.client.mu.Lock()
	delete(c.client.calls, c)
	c.client.mu.Unlock()
}

func (c *remoteCall) readError(err error) error {
	if cause := context.Cause(c.ctx); cause != nil {
		return cause
	}
	if errors.Is(err, encoding.ErrInvalidUTF8) || errors.Is(err, sse.ErrEventTooLarge) || errors.Is(err, sse.ErrLineTooLong) {
		return invalidRemote("invalid or oversized sse frame")
	}
	if errors.Is(err, ErrInvalidResponse) {
		return err
	}
	if problem, ok := errors.AsType[*TransportError](err); ok {
		if problem.StatusCode == http.StatusRequestTimeout || problem.StatusCode == http.StatusTooManyRequests || problem.StatusCode >= http.StatusInternalServerError {
			return errors.Join(ErrDisconnected, err)
		}
		return err
	}
	_, socketError := errors.AsType[*net.OpError](err)
	_, dnsError := errors.AsType[*net.DNSError](err)
	var timeout net.Error
	if socketError || dnsError || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) || (errors.As(err, &timeout) && timeout.Timeout()) {
		return errors.Join(ErrDisconnected, err)
	}
	return err
}
