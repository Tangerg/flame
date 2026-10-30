package main

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"
	"github.com/Tangerg/flame/plugins/memory/internal/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context, data string) (err error) {
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
func main() { backend.Main("memory", run) }
