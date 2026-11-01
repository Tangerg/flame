package main

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/qimu/sqlite"
)

func run(ctx context.Context, data string) (returned error) {
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

func main() { backend.Main("feedback", run) }
