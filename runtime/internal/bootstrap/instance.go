package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/model"
	ownershipadapter "github.com/Tangerg/flame/runtime/internal/adapter/ownership"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/config"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/protocol"
)

// InstanceConfig is the exact host snapshot required to open one complete
// Runtime instance. Callers resolve environment and working-directory defaults
// before this boundary; the instance never rediscovers host paths.
type InstanceConfig struct {
	UserHome             string
	DefaultWorkspacePath string
	DataDirectory        string
	ConfigDirectories    []string
	BuildID              string
	ServerInfo           protocol.ServerInfo
}

// Instance owns one Runtime application and its complete shutdown graph.
// Copies share the same lifecycle owner.
type Instance struct {
	application *runtimeApplication
	serverInfo  protocol.ServerInfo
	lifetime    *runtimeLifetime
}

// OpenInstance serializes canonical data-directory setup, opens persistence,
// then releases that setup boundary before assembling and recovering one Instance.
// Runtime processes may subsequently share the directory; finer application
// ownership prevents conflicting execution and recovery.
func OpenInstance(ctx context.Context, cfg InstanceConfig) (_ *Instance, _ config.Settings, err error) {
	if validateErr := cfg.validate(); validateErr != nil {
		return nil, config.Settings{}, validateErr
	}
	setup, err := ownershipadapter.PrepareDataDirectory(ctx, cfg.DataDirectory)
	if err != nil {
		return nil, config.Settings{}, err
	}
	setupOwned := true
	defer func() {
		if setupOwned {
			err = errors.Join(err, setup.Release())
		}
	}()
	cfg.DataDirectory = setup.Directory

	buildID := cfg.BuildID
	if buildID == "" {
		buildID, err = ExecutableBuildID()
		if err != nil {
			return nil, config.Settings{}, err
		}
	}
	settings, err := LoadConfig(cfg.ConfigDirectories)
	if err != nil {
		return nil, config.Settings{}, err
	}
	stores, err := persistence.Open(ctx, persistence.Config{
		DataDirectory:        cfg.DataDirectory,
		DefaultWorkspacePath: cfg.DefaultWorkspacePath,
	})
	if err != nil {
		return nil, config.Settings{}, err
	}
	idempotencyNamespace := stores.IdempotencyNamespace.String()
	storesOwned := true
	defer func() {
		if storesOwned {
			err = errors.Join(err, stores.Close())
		}
	}()

	if err = SeedConfiguredProvider(ctx, stores.Providers, settings); err != nil {
		return nil, config.Settings{}, err
	}
	providers, err := ProviderRegistry(stores.Providers, settings)
	if err != nil {
		return nil, config.Settings{}, err
	}
	chatResolver, err := modeladapter.NewChatResolver(providers)
	if err != nil {
		return nil, config.Settings{}, err
	}
	if err = SeedUtilityRole(ctx, stores.UtilityRole, settings); err != nil {
		return nil, config.Settings{}, err
	}
	mcpServers, err := MCPServers(settings.MCPServers)
	if err != nil {
		return nil, config.Settings{}, err
	}
	if err = SeedMCPServers(ctx, stores.MCPServers, mcpServers); err != nil {
		return nil, config.Settings{}, err
	}
	if err = setup.Release(); err != nil {
		return nil, config.Settings{}, err
	}
	setupOwned = false

	hookResolver := NewHookResolver(cfg.UserHome, stores.Trust)
	assemblyConfig := ComposeConfig(settings, stores, chatResolver, providers, hookResolver, buildID)
	ownershipLeases, err := ownershipadapter.New(stores.DataDirectory)
	if err != nil {
		return nil, config.Settings{}, err
	}
	assemblyConfig.SessionOwnership = ownershipLeases
	assemblyConfig.GoalDriveOwnership = ownershipLeases
	assemblyConfig.RecoveryOwnership = ownershipLeases
	assemblyConfig.UserHome = cfg.UserHome
	assemblyConfig.DefaultWorkspacePath = cfg.DefaultWorkspacePath
	runtimeContext, stopRuntime := context.WithCancel(context.Background())
	runtimeOwned := true
	defer func() {
		if runtimeOwned {
			stopRuntime()
		}
	}()
	ownedLifetime := newRuntimeLifetime(runtimeContext, assemblyConfig.Resources)
	storesOwned = false
	host, err := assemble(ctx, assemblyConfig, ownedLifetime, buildToolEnvironment)
	if err != nil {
		return nil, config.Settings{}, err
	}
	hostOwned := true
	defer func() {
		if hostOwned {
			err = errors.Join(err, host.Close())
		}
	}()
	if err = host.application.recoverStartup(ctx); err != nil {
		return nil, config.Settings{}, err
	}

	serverInfo := cfg.ServerInfo
	serverInfo.InstanceID = runtimeidentity.NewRuntimeInstance().String()
	if serverInfo.Name == "" {
		serverInfo.Name = runtimeidentity.ProductName
	}
	if serverInfo.Version == "" {
		serverInfo.Version = "dev"
	}
	serverInfo.Home = cfg.UserHome
	serverInfo.DefaultWorkspace = protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}
	endpoint, err := host.application.newDeliveryEndpoint(
		runtimeContext,
		serverInfo,
		idempotencyNamespace,
	)
	if err != nil {
		return nil, config.Settings{}, err
	}
	host.lifetime.delivery = endpoint
	host.lifetime.stopRuntime = stopRuntime
	databaseChangesDone, err := stores.StartExternalChangeObserver(
		runtimeContext,
		host.application.notifyExternalChange,
		func(err error) {
			_, span := otel.Tracer("flame/persistence").Start(runtimeContext, "persistence.external-change.error")
			span.RecordError(err)
			span.SetStatus(codes.Error, "external change observation failed")
			span.End()
		},
	)
	if err != nil {
		return nil, config.Settings{}, err
	}
	workerJoins := host.application.startWorkers(runtimeContext)

	host.serverInfo = serverInfo
	host.lifetime.schedulerDone = workerJoins.scheduler
	host.lifetime.databaseChangesDone = databaseChangesDone
	host.lifetime.recoveryDone = workerJoins.recovery

	runtimeOwned = false
	hostOwned = false
	return host, settings, nil
}

func (i InstanceConfig) validate() error {
	for _, path := range []struct {
		name  string
		value string
	}{
		{name: "user home", value: i.UserHome},
		{name: "default workspace path", value: i.DefaultWorkspacePath},
		{name: "data directory", value: i.DataDirectory},
	} {
		if path.value == "" {
			return fmt.Errorf("runtime: %s is required", path.name)
		}
		if !filepath.IsAbs(path.value) {
			return fmt.Errorf("runtime: %s must be absolute", path.name)
		}
	}
	if len(i.ConfigDirectories) == 0 {
		return errors.New("runtime: at least one config directory is required")
	}
	for _, directory := range i.ConfigDirectories {
		if directory == "" || !filepath.IsAbs(directory) {
			return errors.New("runtime: config directories must be non-empty absolute paths")
		}
	}
	if i.BuildID != "" {
		if _, err := runtimeidentity.ParseBuild(i.BuildID); err != nil {
			return fmt.Errorf("runtime: BuildID: %w", err)
		}
	}
	return nil
}

// Endpoint returns the instance-owned binding-neutral operation entrypoint.
// Public bindings keep it private and expose only their typed methods.
func (i *Instance) Endpoint() *delivery.Endpoint {
	if i == nil || i.lifetime == nil {
		return nil
	}
	return i.lifetime.delivery
}

// ServerInfo returns the immutable identity advertised by every binding.
func (i *Instance) ServerInfo() protocol.ServerInfo {
	if i == nil {
		return protocol.ServerInfo{}
	}
	return i.serverInfo
}
