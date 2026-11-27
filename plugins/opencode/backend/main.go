package main

import (
	"context"
	"errors"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context, data string) (err error) {
	connector, err := openConnector(ctx, data, os.Getenv("OPENCODE_EXECUTABLE"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, connector.close()) }()
	server, err := newServer(connector)
	if err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}

func main() { backend.Main("opencode", run) }
