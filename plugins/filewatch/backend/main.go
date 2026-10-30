package main

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context, data string) (err error) {
	store, err := openStore(ctx, filepath.Join(data, "filewatch.db"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.db.Close()) }()
	server, backend, err := newServer(store)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, backend.Close()) }()
	return server.Run(ctx, &sdk.StdioTransport{})
}

func main() { backend.Main("filewatch", run) }
