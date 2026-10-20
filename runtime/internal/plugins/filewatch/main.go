package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Tangerg/flame/runtime/internal/infra/advisorylock"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context) (err error) {
	data := os.Getenv("PLUGIN_DATA")
	if !filepath.IsAbs(data) {
		return errors.New("filewatch: PLUGIN_DATA must be absolute")
	}
	lease, err := advisorylock.AcquireDirectory(ctx, data)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lease.Release()) }()
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

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
