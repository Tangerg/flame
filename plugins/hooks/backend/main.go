package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/qimu/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context) (returned error) {
	data := os.Getenv("PLUGIN_DATA")
	if !filepath.IsAbs(data) {
		return errors.New("hooks: PLUGIN_DATA must be absolute")
	}
	lease, err := filesystem.AcquireDirectoryLease(ctx, data)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, lease.Release()) }()
	db, err := sqlite.OpenSchema(ctx, filepath.Join(data, "commands.db"), []string{pluginstorage.CommandsSchema})
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, db.Close()) }()
	server, err := newServer(db)
	if err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
