package main

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"
	goalmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/goalworkflow/mcp"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context, _ string) (err error) {
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.goals", Version: "1.0.0"}, nil)
	host, err := pluginmcp.NewBackend(server)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, host.Close()) }()
	method, err := goalmcp.DecideMethod()
	if err != nil {
		return err
	}
	if err := method.RegisterServer(host, nil, func(_ context.Context, _ *sdk.ServerSession, call pluginmcp.Command[goalworkflow.Request]) (goalworkflow.Decision, error) {
		return decide(call.Input), nil
	}); err != nil {
		return err
	}
	contract, err := goalmcp.Contract()
	if err != nil {
		return err
	}
	if err := host.BindCapability("goals", contract); err != nil {
		return err
	}
	if err := host.Publish(); err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}
func main() { backend.Main("goal workflow", run) }
