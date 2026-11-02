package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context, data string) (err error) {
	store, err := openStore(ctx, filepath.Join(data, "providers.db"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	deployments, err := parseAzureDeployments(os.Getenv(azureDeploymentsVariable))
	if err != nil {
		return err
	}
	server, err := newServer(store, deployments)
	if err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}

func main() { backend.Main("model provider", run) }
