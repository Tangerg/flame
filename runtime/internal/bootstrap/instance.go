package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	runtimeidentity "github.com/Tangerg/flame/runtime/identity"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	ownershipadapter "github.com/Tangerg/flame/runtime/internal/adapter/ownership"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/config"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// InstanceConfig is the exact host snapshot required to open one complete
// Runtime instance. Callers resolve environment and working-directory defaults
// before this boundary; the instance never rediscovers host paths.
type InstanceConfig struct {
	UserHome             string
	DefaultWorkspacePath string
	DataDirectory        string
	FirstPartyDirectory  string
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
	stores      *persistence.Bundle
}

// OpenInstance exclusively owns the data directory through complete shutdown.
func OpenInstance(ctx context.Context, cfg InstanceConfig) (_ *Instance, _ config.Settings, err error) {
	if validateErr := cfg.validate(); validateErr != nil {
		return nil, config.Settings{}, validateErr
	}
	directory, err := ownershipadapter.OpenDataDirectory(ctx, cfg.DataDirectory)
	if err != nil {
		return nil, config.Settings{}, err
	}
	directoryOwned := true
	defer func() {
		if directoryOwned {
			err = errors.Join(err, directory.Close())
		}
	}()
	cfg.DataDirectory = directory.Directory

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
	stores, err := persistence.Open(ctx, persistence.Config{DataDirectory: cfg.DataDirectory, ValidateDirectory: directory.Validate})
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

	if err = initializeFirstPartyPlugins(ctx, stores, cfg.FirstPartyDirectory); err != nil {
		return nil, config.Settings{}, err
	}

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
	assemblyConfig := ComposeConfig(settings, stores, chatResolver, providers, buildID)
	// Resources close in reverse acquisition order. The directory must remain
	// leased even when a constructor's bounded shutdown wait expires.
	assemblyConfig.Resources = append([]TerminalResource{directory}, assemblyConfig.Resources...)
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
	directoryOwned = false
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
		directory.Validate,
	)
	if err != nil {
		return nil, config.Settings{}, err
	}
	host.lifetime.delivery = endpoint
	host.lifetime.stopRuntime = stopRuntime
	// The delivery endpoint published the callback RPC, so background plugins can
	// now activate; only now may uncertain-execution revival query their sessions.
	if err = host.application.recoverUncertainExecutions(ctx); err != nil {
		return nil, config.Settings{}, err
	}
	workerJoins := host.application.startWorkers(runtimeContext)

	host.serverInfo = serverInfo
	host.stores = stores
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
		{name: "first-party directory", value: i.FirstPartyDirectory},
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
	return i.lifetime.delivery
}

// ServerInfo returns the immutable identity advertised by every binding.
func (i *Instance) ServerInfo() protocol.ServerInfo { return i.serverInfo }

// CheckStorage reports whether durable storage can still answer. ctx's error is
// returned unwrapped when the single-connection pool is busy, so a caller can
// distinguish a saturated runtime from a broken one.
func (i *Instance) CheckStorage(ctx context.Context) error { return i.stores.Ping(ctx) }
