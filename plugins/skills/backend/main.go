package main

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context, data string) (err error) {
	learner, err := openLearner(ctx, filepath.Join(data, "learning.db"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, learner.db.Close()) }()
	server, err := newServer(learner)
	if err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}

func main() { backend.Main("skill learning", run) }
