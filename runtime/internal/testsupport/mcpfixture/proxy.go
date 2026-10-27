// Package mcpfixture supplies transport-only helpers for process integration tests.
package mcpfixture

import (
	"context"
	"errors"
	"io"

	sdk "github.com/Tangerg/go-sdk/mcp"
)

// Forward bridges the original SDK transports without interpreting lifecycle,
// contracts or business results. The upstream must implement its own protocol.
func Forward(parent context.Context, endpoint string) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	upstream, err := (&sdk.StreamableClientTransport{Endpoint: endpoint, MaxRetries: -1}).Connect(ctx)
	if err != nil {
		return err
	}
	defer upstream.Close()
	stdio, err := (&sdk.StdioTransport{}).Connect(ctx)
	if err != nil {
		return err
	}
	defer stdio.Close()
	finished := make(chan error, 2)
	forward := func(source, target sdk.Connection) {
		for {
			message, err := source.Read(ctx)
			if err != nil {
				finished <- err
				return
			}
			if err := target.Write(ctx, message); err != nil {
				finished <- err
				return
			}
		}
	}
	go forward(stdio, upstream)
	go forward(upstream, stdio)
	err = <-finished
	cancel()
	_ = upstream.Close()
	_ = stdio.Close()
	<-finished
	if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
