package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	mcpadapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	browseradapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginbrowser"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginpackage"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginservices"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"

	"github.com/Tangerg/frame"
	framebrowser "github.com/Tangerg/frame/browser"
	"github.com/Tangerg/frame/distribution"
	framehost "github.com/Tangerg/frame/host"
	"github.com/Tangerg/frame/process"
	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

// Framework capacities: how many installations the host admits, how long a
// plugin process may take to stop, and what the browser bridge holds at once.
const (
	maxInstallations          = 128
	pluginProcessCloseTimeout = 10 * time.Second
	browserRealmCapacity      = 64
	browserPendingCapacity    = 4096
	browserProofCapacity      = 4096
	browserMessageBytes       = 4 << 20
	browserPollInterval       = 100 * time.Millisecond
)

type frameworkComposition struct {
	host      framehost.Host
	process   *process.Driver
	installed *mcpadapter.Installed
	registry  *plugins.Registry
	external  *mcpadapter.ExternalConnector
	browser   *browseradapter.Service
	control   http.Handler
	commands  *plugins.Commands
	callers   *pluginservices.CallerRestorer
	ready     <-chan struct{}
}

func buildPluginPackages(cfg assemblyConfig, lifetime *runtimeLifetime) (*pluginpackage.Releases, error) {
	content, err := distribution.Open(distribution.Config{
		Directory: filepath.Join(cfg.stores.DataDirectory, "plugins", "frame"),
		Admission: cfg.stores.Admission,
		Trust: func(_ context.Context, source distribution.Source, _ distribution.Release) error {
			// Flame installs explicitly selected local packages. Remote download
			// and publisher authentication require a distinct product policy.
			if !filepath.IsAbs(source.Location) {
				return errors.New("plugins: an explicitly selected local package is required")
			}
			return nil
		},
		Retained: func(ctx context.Context, digest framevalue.Digest) (bool, error) {
			return retainedPluginContent(ctx, cfg.stores, digest)
		},
	})
	if err != nil {
		return nil, err
	}
	lifetime.toolResources = append(lifetime.toolResources, terminalStep(content.Close))
	return pluginpackage.New(content, filepath.Join(cfg.stores.DataDirectory, "plugins", "data"), cfg.stores.PluginReleases)
}

// Remove invokes this reader while holding the same admission as every writer
// that can add a reference. The original business rows remain the authority.
func retainedPluginContent(ctx context.Context, stores *persistence.Bundle, digest framevalue.Digest) (bool, error) {
	installations, err := stores.Installations.List(ctx)
	if err != nil {
		return false, err
	}
	for _, snapshot := range installations {
		record := snapshot.Installation.Snapshot()
		if record.Selected == digest || record.Staged != nil && *record.Staged == digest {
			return true, nil
		}
	}
	checkpoints, err := stores.ExecutorCheckpoints.PendingCheckpointDependencies(ctx)
	if err != nil {
		return false, err
	}
	commands, err := stores.PluginCommands.PendingCommandDependencies(ctx)
	if err != nil {
		return false, err
	}
	for _, dependency := range append(checkpoints, commands...) {
		if dependency.Digest == digest {
			return true, nil
		}
	}
	executions, err := stores.Frame.ExecutionDependencies(ctx)
	if err != nil {
		return false, err
	}
	for _, dependency := range executions {
		if dependency.Release == digest {
			return true, nil
		}
	}
	return false, nil
}

// buildBrowser registers the browser bridge as a host driver and the plugin
// browser that issues realms through it.
func buildBrowser(host framehost.Host, stores *persistence.Bundle, packages *pluginpackage.Releases, registry *plugins.Registry) (*framebrowser.Bridge, *browseradapter.Service, error) {
	source, err := browseradapter.NewSource(stores.Installations, packages)
	if err != nil {
		return nil, nil, err
	}
	bridge, err := framebrowser.NewBridge(framebrowser.BridgeConfig{Host: host, Store: stores.Frame, Source: source, PublicConfiguration: source.Configuration, MaxRealms: browserRealmCapacity, MaxPending: browserPendingCapacity, MaxMessageBytes: browserMessageBytes})
	if err != nil {
		return nil, nil, err
	}
	if err := host.RegisterDriver(bridge); err != nil {
		return nil, nil, err
	}
	browser, err := browseradapter.New(browseradapter.Config{Bridge: bridge, Registry: registry, ControlEndpoint: flamehttp.PluginControlPath, ProofCapacity: browserProofCapacity, PreparationCapacity: browserPendingCapacity, PollInterval: browserPollInterval})
	if err != nil {
		return nil, nil, err
	}
	return bridge, browser, nil
}

// startFrameworkHost starts the plugin host under owner and registers its
// teardown with the tool resources.
func startFrameworkHost(ctx context.Context, stores *persistence.Bundle, lifetime *runtimeLifetime, owner frame.Lifetime, permission *mcpadapter.CatalogPermission, profile *process.Driver) (framehost.Host, error) {
	lifetime.frameworkCauses = owner.ObserveCloseCauses()
	host, err := framehost.New(ctx, framehost.Config{ReadAcceptedBinding: permission.Read, Authority: stores.Frame, Lifetime: owner, Limits: framehost.Limits{MaxInstallations: maxInstallations}, Drivers: []framehost.Driver{profile}})
	if err != nil {
		return framehost.Host{}, err
	}
	lifetime.toolResources = append(lifetime.toolResources, terminalStep(func(ctx context.Context) error {
		_, diagnostic := host.Close(ctx)
		// An unconfirmed resource must keep the SQLite authority and content
		// protected. Caller deadlines only stop their wait for this generation.
		_, releaseErr := owner.BeginClose(frame.ErrClosed).WaitReleased(ctx)
		return errors.Join(diagnostic, releaseErr)
	}))
	return host, nil
}

// buildMCPConnectors builds the installed-plugin connector and the external
// connector that shares its catalog.
func buildMCPConnectors(stores *persistence.Bundle, config mcpadapter.InstalledConfig) (*mcpadapter.Installed, *mcpadapter.ExternalConnector, error) {
	installed, err := mcpadapter.NewInstalled(config)
	if err != nil {
		return nil, nil, err
	}
	external, err := mcpadapter.NewExternalConnector(installed, stores.Installations, stores.ProviderSelections, stores.MCPServers)
	if err != nil {
		return nil, nil, err
	}
	return installed, external, nil
}

func buildFramework(ctx context.Context, cfg assemblyConfig, lifetime *runtimeLifetime, packages *pluginpackage.Releases, callbacks *pluginmcp.Callbacks) (frameworkComposition, error) {
	commands, err := plugins.NewCommands(cfg.stores.PluginCommands, cfg.stores.Admission)
	if err != nil {
		return frameworkComposition{}, err
	}
	callers, err := pluginservices.NewCallerRestorer(commands, cfg.stores.ToolInvocations)
	if err != nil {
		return frameworkComposition{}, err
	}
	source, err := mcpadapter.NewProcessSource(cfg.stores.Installations, packages, callbacks, callers)
	if err != nil {
		return frameworkComposition{}, err
	}
	profile, err := process.New(process.Config{Runner: cfg.processRunner, Store: cfg.stores.Frame, Source: source, CloseTimeout: pluginProcessCloseTimeout, Protocol: source, MaxFrameBytes: sdk.DefaultMaxLineLength})
	if err != nil {
		return frameworkComposition{}, err
	}
	permission, err := mcpadapter.NewCatalogPermission(cfg.stores.Admission, cfg.stores.PendingPluginBindings)
	if err != nil {
		return frameworkComposition{}, err
	}
	owner := frame.NewLifetime(context.WithoutCancel(lifetime.context))
	host, err := startFrameworkHost(ctx, cfg.stores, lifetime, owner, permission, profile)
	if err != nil {
		return frameworkComposition{}, err
	}
	registry, err := plugins.NewRegistry(host, cfg.stores.Installations, packages, cfg.stores.ProviderSelections, cfg.stores.PluginCompositions)
	if err != nil {
		return frameworkComposition{}, err
	}
	bridge, browser, err := buildBrowser(host, cfg.stores, packages, registry)
	if err != nil {
		return frameworkComposition{}, err
	}
	if err := restoreFramework(ctx, cfg.stores, host, profile.Profile()); err != nil {
		return frameworkComposition{}, err
	}
	recovery := &frameworkRecovery{stores: cfg.stores, process: profile, pending: true, ready: make(chan struct{})}
	if err := recovery.restore(ctx); err != nil && !errors.Is(err, framehost.ErrRecoveryRequired) {
		return frameworkComposition{}, err
	}
	if cfg.firstPartyDirectory != "" {
		if err := initializeFirstPartyPlugins(ctx, cfg.stores, host, packages, cfg.firstPartyDirectory); err != nil {
			return frameworkComposition{}, err
		}
	}
	installed, external, err := buildMCPConnectors(cfg.stores, mcpadapter.InstalledConfig{
		Host: host, Source: source, Registry: registry, Lifetime: owner,
		Permission: permission, Ready: recovery.ready,
	})
	if err != nil {
		return frameworkComposition{}, err
	}
	control := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		bridge.Handler().ServeHTTP(writer, request)
		// Original cleanup proofs can arrive while recovery admission is shut.
		// Only complete reconstruction of every retained owner opens it again.
		_ = recovery.restore(request.Context())
	})
	return frameworkComposition{host: host, process: profile, registry: registry, external: external, installed: installed, browser: browser, control: control, commands: commands, callers: callers, ready: recovery.ready}, nil
}

func restoreFramework(ctx context.Context, stores *persistence.Bundle, host framehost.Host, backendProfile framehost.Profile) error {
	records, err := stores.Frame.RecoverInstallations(ctx)
	if err != nil {
		return err
	}
	definitions := make([]*framehost.Plugin, 0, len(records))
	for _, record := range records {
		digest := record.Release
		if err := digest.Validate(); err != nil {
			return err
		}
		release, err := stores.PluginReleases.Get(ctx, digest)
		if err != nil {
			return err
		}
		projection, err := plugins.ProjectFrameworkRelease(release, backendProfile)
		if err != nil {
			return err
		}
		if err := plugins.RegisterFrameworkRelease(host, projection); err != nil {
			return err
		}
		definitions = append(definitions, projection.Plugin)
	}
	if err := host.Restore(ctx, definitions); err != nil {
		return fmt.Errorf("plugins: restore original framework records: %w", err)
	}
	return stores.RecoverPluginBindings(ctx)
}

type frameworkRecovery struct {
	mu      sync.Mutex
	stores  *persistence.Bundle
	process *process.Driver
	pending bool
	ready   chan struct{}
}

func (r *frameworkRecovery) restore(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.pending {
		return nil
	}
	if err := r.process.Recover(ctx); err != nil {
		r.stores.Admission.RequireRecovery(err)
		return errors.Join(framehost.ErrRecoveryRequired, err)
	}
	err := r.stores.Admission.Recover(ctx, func() error {
		pending, err := r.stores.PluginUses.Pending(ctx)
		if err != nil {
			return err
		}
		if len(pending) != 0 {
			return framehost.ErrRecoveryRequired
		}
		dependencies, err := r.stores.Frame.ExecutionDependencies(ctx)
		if err != nil {
			return err
		}
		if len(dependencies) != 0 {
			return framehost.ErrRecoveryRequired
		}
		return nil
	})
	if err == nil {
		r.pending = false
		close(r.ready)
	}
	return err
}
