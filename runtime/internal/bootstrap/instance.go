package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/config"
	"github.com/Tangerg/flame/runtime/internal/delivery"

	framehost "github.com/Tangerg/frame/host"
	"github.com/Tangerg/frame/process/runner"
)

// InstanceConfig is the exact host snapshot required to open one complete
// Runtime instance. Callers resolve environment and working-directory defaults
// before this boundary; the instance never rediscovers host paths.
type InstanceConfig struct {
	UserHome             string
	DefaultWorkspacePath string
	DataDirectory        string
	FirstPartyDirectory  string
	RunnerExecutable     string
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
	directory, err := openDataDirectory(ctx, cfg.DataDirectory)
	if err != nil {
		return nil, config.Settings{}, err
	}
	directoryOwned := true
	defer func() {
		if directoryOwned {
			err = errors.Join(err, directory.Close())
		}
	}()
	cfg.DataDirectory = directory.directory

	buildID, err := cfg.buildID()
	if err != nil {
		return nil, config.Settings{}, err
	}
	settings, err := config.Load(cfg.ConfigDirectories)
	if err != nil {
		return nil, config.Settings{}, err
	}
	stores, err := persistence.Open(ctx, persistence.Config{DataDirectory: cfg.DataDirectory, ValidateDirectory: directory.validate})
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

	providers, err := seedSettings(ctx, stores, settings)
	if err != nil {
		return nil, config.Settings{}, err
	}
	processRunner, err := runner.NewDaemon(runner.DaemonConfig{Executable: cfg.RunnerExecutable, Directory: filepath.Join(cfg.DataDirectory, "runner"), StartTimeout: runnerStartTimeout})
	if err != nil {
		return nil, config.Settings{}, err
	}
	assemblyConfig := composeConfig(settings, stores, providers, buildID)
	assemblyConfig.processRunner = processRunner
	assemblyConfig.firstPartyDirectory = cfg.FirstPartyDirectory
	// Resources close in reverse acquisition order. The directory must remain
	// leased even when a constructor's bounded shutdown wait expires.
	assemblyConfig.resources = append([]terminalResource{directory}, assemblyConfig.resources...)
	assemblyConfig.userHome = cfg.UserHome
	assemblyConfig.defaultWorkspacePath = cfg.DefaultWorkspacePath
	runtimeContext, stopRuntime := context.WithCancel(context.Background())
	runtimeOwned := true
	defer func() {
		if runtimeOwned {
			stopRuntime()
		}
	}()
	ownedLifetime := newRuntimeLifetime(runtimeContext, assemblyConfig.resources)
	ownedLifetime.toolResources = append(ownedLifetime.toolResources, terminalStep(func(ctx context.Context) error { return processRunner.Close(ctx) }))
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
	if err := host.start(ctx, runtimeContext, stopRuntime, cfg.serverInfo(), idempotencyNamespace, directory.validate); err != nil {
		return nil, config.Settings{}, err
	}
	host.stores = stores

	runtimeOwned = false
	hostOwned = false
	return host, settings, nil
}

func (c InstanceConfig) buildID() (string, error) {
	if c.BuildID != "" {
		return c.BuildID, nil
	}
	return ExecutableBuildID()
}

// start recovers durable work, opens delivery and starts the workers, in the
// order each step needs the previous one.
func (h *Instance) start(ctx, runtimeContext context.Context, stopRuntime context.CancelFunc, serverInfo protocol.ServerInfo, idempotencyNamespace string, validateDirectory func() error) error {
	if err := h.application.recoverStartup(ctx); err != nil {
		return err
	}
	endpoint, err := h.application.newDeliveryEndpoint(runtimeContext, serverInfo, idempotencyNamespace, validateDirectory)
	if err != nil {
		return err
	}
	h.lifetime.delivery = endpoint
	h.lifetime.stopRuntime = stopRuntime
	// The delivery endpoint published the callback RPC, so background plugins can
	// now activate; only now may uncertain-execution revival query their sessions.
	if err := h.application.recoverUncertainExecutions(ctx); err != nil && !startupRecoveryDeferred(err) {
		return err
	}
	workerJoins := h.application.startWorkers(runtimeContext)
	h.serverInfo = serverInfo
	h.lifetime.recoveryDone = workerJoins.recovery
	return nil
}

// runnerStartTimeout bounds how long startup waits for the sandbox runner
// daemon before failing instead of hanging.
const runnerStartTimeout = 10 * time.Second

// developmentVersion names a build that was not stamped with a release.
const developmentVersion = "dev"

// unstampedModuleVersion is Go's build-info placeholder for a main module
// built from a working tree rather than a tagged module.
const unstampedModuleVersion = "(devel)"

// BuildVersion is this binary's module version, or developmentVersion for a
// local build.
func BuildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == unstampedModuleVersion {
		return developmentVersion
	}
	return info.Main.Version
}

// seedSettings applies configured providers and roles to the store and
// returns the provider registry they define.
func seedSettings(ctx context.Context, stores *persistence.Bundle, settings config.Settings) (models.ProviderRegistry, error) {
	if err := seedConfiguredProvider(ctx, stores.Providers, settings); err != nil {
		return nil, err
	}
	providers, err := providerRegistry(stores.Providers, settings)
	if err != nil {
		return nil, err
	}
	if err := seedUtilityRole(ctx, stores.UtilityRole, settings); err != nil {
		return nil, err
	}
	return providers, nil
}

func (i InstanceConfig) serverInfo() protocol.ServerInfo {
	info := i.ServerInfo
	info.InstanceID = delivery.NewInstanceID()
	if info.Name == "" {
		info.Name = identity.ProductName
	}
	if info.Version == "" {
		info.Version = BuildVersion()
	}
	info.Home = i.UserHome
	info.DefaultWorkspace = protocol.WorkspaceRef{Path: i.DefaultWorkspacePath}
	return info
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
		{name: "runner executable", value: i.RunnerExecutable},
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
		if _, err := runs.ParseBuild(i.BuildID); err != nil {
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

// PluginControl returns the authenticated Realm protocol endpoint. It remains
// available for original cleanup proofs while application admission is closed.
func (i *Instance) PluginControl() http.Handler { return i.application.pluginControl }

func (i *Instance) CheckPlugins(context.Context) error { return i.stores.Admission.RecoveryError() }

func startupRecoveryDeferred(err error) bool {
	return errors.Is(err, framehost.ErrRecoveryRequired) || errors.Is(err, runs.ErrOriginalSourcePending)
}
