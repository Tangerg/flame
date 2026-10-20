package main

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/plugins/usagereports/backend/rpc"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	pluginstorage "github.com/Tangerg/flame/runtime/plugin/sqlite"
	"github.com/Tangerg/flame/runtime/protocol"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func newServer(archive *archive) (*sdk.Server, error) {
	if archive == nil {
		return nil, errors.New("usage reports: archive is required")
	}
	contracts, err := rpc.NewContracts[protocol.UsageSummary]()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.usagereports", Version: "1.0.0"}, nil)
	backend, err := pluginsdk.NewBackend(server)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, archive.store.db),
		contracts.Capture.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[rpc.CaptureInput]) (rpc.Header, error) {
			original, err := command.RequestInput()
			if err != nil {
				return rpc.Header{}, err
			}
			return archive.capture(ctx, contracts.Capture.Name(), command.ID, command.Input, original)
		}),
		contracts.List.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, _ pluginsdk.Command[struct{}]) ([]rpc.Header, error) {
			return archive.store.list(ctx)
		}),
		contracts.Get.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[rpc.ReportInput]) (rpc.Report[protocol.UsageSummary], error) {
			return archive.store.read(ctx, command.Input.ID)
		}),
		pluginstorage.RegisterCommand(backend, archive.store.db, contracts.Delete, classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ReportInput) (bool, error) {
			return archive.store.delete(ctx, input.ID)
		}),
		backend.BindCapability("reports", contracts.Capability),
	); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}
