package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Tangerg/flame/plugins/schedules/backend/sqlite"
	"github.com/Tangerg/flame/plugins/schedules/backend/transport"
	models "github.com/Tangerg/flame/packages/go-sdk/model/catalog"
	"github.com/Tangerg/flame/packages/go-sdk/storage/advisorylock"
	"github.com/Tangerg/flame/packages/go-sdk/storage/directories"
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
	backend, err := transport.RegisterServer(server, db, sqlite.NewScheduleStore(db), directories.Resolver{}, models.Selections{}, connectRunHost)
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
