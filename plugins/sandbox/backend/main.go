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
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context) (err error) {
	data := os.Getenv("PLUGIN_DATA")
	if !filepath.IsAbs(data) {
		return errors.New("sandbox: PLUGIN_DATA must be absolute")
	}
	lease, err := filesystem.AcquireDirectoryLease(ctx, data)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lease.Release()) }()
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

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
