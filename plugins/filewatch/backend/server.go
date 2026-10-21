package main

import (
	"context"
	"errors"
	"io"
	"sync"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	pluginstorage "github.com/Tangerg/flame/packages/plugin-sdk/sqlite"
	filewatch "github.com/Tangerg/flame/plugins/filewatch/backend/domain"
	"github.com/Tangerg/flame/plugins/filewatch/backend/rpc"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func newServer(store *store) (*sdk.Server, *pluginsdk.Backend, error) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.filewatch", Version: "1.0.0"}, nil)
	backend, err := pluginsdk.NewBackend(server)
	if err != nil {
		return nil, nil, err
	}
	var activation struct {
		sync.Mutex
		worker *background
	}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, store.db),
		pluginstorage.RegisterCommand(backend, store.db, contracts.Configure, classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ConfigureInput) (filewatch.Snapshot, error) {
			return store.configure(ctx, input.ExpectedRevision, input.Trigger)
		}),
		contracts.Get.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, _ pluginsdk.Command[struct{}]) (filewatch.Snapshot, error) {
			configuration, err := store.configuration(ctx)
			return configuration.Snapshot(), err
		}),
		contracts.Events.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, _ pluginsdk.Command[struct{}]) ([]filewatch.Header, error) {
			return store.headers(ctx)
		}),
		contracts.Event.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[rpc.EventInput]) (filewatch.Event, error) {
			return store.event(ctx, command.Input.ID)
		}),
		contracts.Status.RegisterServer(backend, classify, func(context.Context, *sdk.ServerSession, pluginsdk.Command[struct{}]) (filewatch.Status, error) {
			activation.Lock()
			defer activation.Unlock()
			if activation.worker == nil {
				return filewatch.Status{Phase: filewatch.Inactive}, nil
			}
			return activation.worker.snapshot(), nil
		}),
		backend.BindCapability("filewatch", contracts.Capability),
	); err != nil {
		return nil, nil, err
	}
	if err := backend.OnSetup(func(ctx context.Context, setup pluginsdk.Setup) (io.Closer, error) {
		values, err := setup.Contributions()
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			if value.Export != "filewatch" || value.Background == nil {
				continue
			}
			ref, err := pluginsdk.CallbackFromContext(ctx)
			if err != nil {
				return nil, err
			}
			host, err := connectHost(ctx, ref)
			if err != nil {
				return nil, err
			}
			namespace, err := host.Namespace(ctx)
			if err == nil {
				err = pluginstorage.CheckRuntimeTarget(ctx, store.db, namespace)
			}
			if err != nil {
				return nil, errors.Join(err, host.client.Close())
			}
			worker := newBackground(ctx, store, host, namespace, *value.Background, value.ID, host.client.Close)
			activation.Lock()
			activation.worker = worker
			activation.Unlock()
			return worker, nil
		}
		return nil, errors.New("filewatch: declared background contribution is required")
	}); err != nil {
		return nil, nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, nil, err
	}
	return server, backend, nil
}

func classify(err error) string {
	if errors.Is(err, errStored) {
		return ""
	}
	return rpc.Classify(err)
}
