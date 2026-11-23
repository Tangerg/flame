package main

import (
	"context"
	"errors"
	"io"
	"sync"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/filewatch/internal/contract"
	"github.com/Tangerg/flame/plugins/filewatch/internal/filewatch"
	"github.com/Tangerg/flame/plugins/filewatch/internal/wire"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const capabilityExport = "filewatch"

func newServer(store *store) (*sdk.Server, *pluginmcp.Backend, error) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.filewatch", Version: "1.0.0"}, nil)
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		return nil, nil, err
	}
	active := &activeWatcher{store: store}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, store.db),
		pluginstorage.RegisterCommand(backend, store.db, contracts.Configure, classify, func(ctx context.Context, _ *sdk.ServerSession, input wire.ConfigureInput) (pluginstorage.CommandPlan[filewatch.Snapshot], error) {
			return store.prepareConfiguration(ctx, input.ExpectedRevision, input.Trigger)
		}),
		contracts.Get.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, _ pluginmcp.Command[struct{}]) (filewatch.Snapshot, error) {
			configuration, err := store.configuration(ctx)
			return configuration.Snapshot(), err
		}),
		contracts.Events.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, _ pluginmcp.Command[struct{}]) ([]filewatch.Header, error) {
			return store.headers(ctx)
		}),
		contracts.Event.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[wire.EventInput]) (filewatch.Event, error) {
			return store.event(ctx, command.Input.ID)
		}),
		contracts.Status.RegisterServer(backend, classify, func(context.Context, *sdk.ServerSession, pluginmcp.Command[struct{}]) (filewatch.Status, error) {
			return active.status(), nil
		}),
		backend.BindCapability(capabilityExport, contracts.Capability),
		backend.OnSetup(active.setUp),
	); err != nil {
		return nil, nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, nil, err
	}
	return server, backend, nil
}

type activeWatcher struct {
	store   *store
	mu      sync.Mutex
	watcher *watcher
}

func (a *activeWatcher) status() filewatch.Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.watcher == nil {
		return filewatch.Status{Phase: filewatch.Inactive}
	}
	return a.watcher.snapshot()
}

func (a *activeWatcher) setUp(ctx context.Context, setup pluginmcp.Setup) (io.Closer, error) {
	values := setup.Contributions()
	for _, value := range values {
		if value.Export == capabilityExport && value.Background != nil {
			return a.start(ctx, value)
		}
	}
	return nil, errors.New("filewatch: declared background contribution is required")
}

func (a *activeWatcher) start(ctx context.Context, value pluginmcp.SetupContribution) (io.Closer, error) {
	ref, err := pluginmcp.CallbackFromContext(ctx)
	if err != nil {
		return nil, err
	}
	host, err := connectHost(ctx, ref)
	if err != nil {
		return nil, err
	}
	namespace, err := host.Namespace(ctx)
	if err == nil {
		err = pluginstorage.CheckRuntimeTarget(ctx, a.store.db, namespace)
	}
	if err != nil {
		return nil, errors.Join(err, host.client.Close())
	}
	watcher := newWatcher(a.store, host, namespace, *value.Background, value.ID)
	background, err := watcher.background(ctx, host.client.Close)
	if err != nil {
		return nil, errors.Join(err, host.client.Close())
	}
	a.mu.Lock()
	a.watcher = watcher
	a.mu.Unlock()
	return background, nil
}
