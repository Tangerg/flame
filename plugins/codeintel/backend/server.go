package main

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/codeintel"
	"github.com/Tangerg/flame/plugins/codeintel/internal/analyzer"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func newServer(analysis *analyzer.Analyzer) (*sdk.Server, error) {
	if analysis == nil {
		return nil, errors.New("code intelligence: analyzer is required")
	}
	contract, err := codeintel.Contract()
	if err != nil {
		return nil, err
	}
	format, err := codeintel.FormatMethod()
	if err != nil {
		return nil, err
	}
	inspect, err := codeintel.InspectMethod()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.codeintel", Version: "1.0.0"}, nil)
	backend, err := qimumcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	lsp, err := newQuery(analysis)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		pluginsdk.RegisterTools(server, lsp),
		format.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[codeintel.Document]) (codeintel.Transformation, error) {
			return formatDocument(ctx, command.Input)
		}),
		inspect.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[codeintel.Inspection]) (codeintel.Report, error) {
			return analysis.Inspect(ctx, command.Input)
		}),
		backend.BindCapability("code-intelligence", contract),
	); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}
