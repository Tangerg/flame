package main

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"

	"github.com/Tangerg/flame/plugins/schedules/internal/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context, data string) (err error) {
	db, err := sqlite.Open(ctx, filepath.Join(data, "schedules.db"))
	if err != nil {
		return errors.Join(err, db.Close())
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.schedules", Version: "3.0.0"}, nil)
	backend, err := registerServer(server, db, sqlite.NewScheduleStore(db), filesystem.DirectoryResolver{}, connectRunHost)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, backend.Close()) }()
	return server.Run(ctx, &sdk.StdioTransport{})
}
func main() { backend.Main("schedules", run) }
