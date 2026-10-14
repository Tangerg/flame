package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	workspace "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/advisorylock"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/sqlite"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/transport"
	models "github.com/Tangerg/flame/runtime/model/catalog"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context) (err error) {
	data := os.Getenv("PLUGIN_DATA")
	if !filepath.IsAbs(data) {
		return errors.New("schedules: PLUGIN_DATA must be absolute")
	}
	lease, err := advisorylock.AcquireDirectory(ctx, data)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lease.Release()) }()
	db, err := sqlite.Open(ctx, filepath.Join(data, "schedules.db"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.schedules", Version: "3.0.0"}, nil)
	backend, err := transport.RegisterServer(server, db, sqlite.NewScheduleStore(db), workspace.Resolver{}, models.Selections{}, connectRunHost)
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
