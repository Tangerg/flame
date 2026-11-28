package main

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/frame/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context, directory string) (err error) {
	db, err := sqlite.OpenSchema(ctx, filepath.Join(directory, "methods.db"), []string{pluginstorage.CommandsSchema, methodSchema})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	worker := &connector{store: store{db}, wake: make(chan struct{}, 1)}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.methods", Version: "1.0.0"}, nil)
	operations, err := register(server, worker)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, operations.Close()) }()
	return server.Run(ctx, &sdk.StdioTransport{})
}

func main() { backend.Main("methods", run) }
