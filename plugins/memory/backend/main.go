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
	"github.com/Tangerg/flame/plugins/memory/internal/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context) (err error) {
	data := os.Getenv("PLUGIN_DATA")
	if !filepath.IsAbs(data) {
		return errors.New("memory: PLUGIN_DATA must be absolute")
	}
	lease, err := filesystem.AcquireDirectoryLease(ctx, data)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lease.Release()) }()
	db, err := sqlite.Open(ctx, filepath.Join(data, "memory.db"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.memory", Version: "3.0.0"}, nil)
	if err := registerServer(server, db, sqlite.NewAgentMemoryStore(db), filesystem.DirectoryResolver{}); err != nil {
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
