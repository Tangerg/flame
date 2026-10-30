package main

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context, data string) (err error) {
	db, err := openStore(ctx, filepath.Join(data, "sandbox.db"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	runner, err := newRunner(db, filepath.Join(data, "scratch"), coreHome)
	if err != nil {
		return err
	}
	server, err := newServer(runner)
	if err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}

func main() { backend.Main("sandbox", run) }
