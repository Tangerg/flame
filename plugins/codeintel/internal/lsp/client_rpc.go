package lsp

import (
	"context"
	"errors"

	"github.com/sourcegraph/jsonrpc2"
)

func (c *client) call(ctx context.Context, method string, params, result any) error {
	var pending jsonrpc2.Waiter
	err := c.send(ctx, func() error {
		var err error
		pending, err = c.conn.DispatchCall(ctx, method, params)
		return err
	})
	if err != nil {
		return err
	}
	err = pending.Wait(ctx, result)
	if err != nil && errors.Is(err, ctx.Err()) {
		return errors.Join(err, context.Cause(ctx))
	}
	return err
}

func (c *client) notify(ctx context.Context, method string, params any) error {
	return c.send(ctx, func() error { return c.conn.Notify(ctx, method, params) })
}

// jsonrpc2 does not observe context while writing. An interrupted frame cannot
// be reused; close its connection and join cleanup before returning its cause.
// The guard ends at dispatch, so canceling a response wait keeps a healthy server.
func (c *client) send(ctx context.Context, send func() error) error {
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return err
	}
	closed := make(chan struct{})
	var closeErr error
	stop := context.AfterFunc(ctx, func() {
		closeErr = c.conn.Close()
		close(closed)
	})
	defer stop()
	err := send()
	if stop() {
		return err
	}
	<-closed
	return errors.Join(err, ctx.Err(), context.Cause(ctx), closeErr)
}
