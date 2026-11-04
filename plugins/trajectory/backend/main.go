package main

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context, _ string) (err error) {
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.trajectory", Version: "4.0.0"}, nil)
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, backend.Close()) }()
	render, err := plugin.RenderSessionMarkdownMethod()
	if err != nil {
		return err
	}
	if err = render.RegisterServer(backend, renderMarkdown); err != nil {
		return err
	}
	contract, err := plugin.SessionMarkdownContract()
	if err != nil {
		return err
	}
	if err = backend.BindCapability("markdown", contract); err != nil {
		return err
	}
	if err = backend.Publish(); err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}
func main() { backend.Main("session export", run) }
