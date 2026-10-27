package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	controlhost "github.com/Tangerg/flame/qimu/control/host"
	"github.com/Tangerg/flame/qimu/distribution"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/process"
	mcpadapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	browseradapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginbrowser"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginpackage"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginservices"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type frameworkComposition struct {
	host      *qimu.Host
	process   *process.Driver
	installed *mcpadapter.Installed
	registry  *plugins.Registry
	browser   *browseradapter.Service
	control   http.Handler
	commands  *plugins.Commands
	callers   *pluginservices.CallerRestorer
	ready     <-chan struct{}
}

func buildPluginPackages(cfg assemblyConfig, lifetime *runtimeLifetime) (*pluginpackage.Releases, error) {
	content, err := distribution.Open(distribution.Config{
		Directory: filepath.Join(cfg.stores.DataDirectory, "plugins", "qimu"),
		Admission: cfg.stores.Admission,
		Trust: func(_ context.Context, source distribution.Source, _ distribution.Manifest, _ distribution.Lock) error {
			// Flame installs explicitly selected local packages. Remote download
			// and publisher authentication require a distinct product policy.
			if !filepath.IsAbs(source.Location) {
				return errors.New("plugins: an explicitly selected local package is required")
			}
			return nil
		},
		Retained: func(ctx context.Context, digest qimu.Digest) (bool, error) {
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
func retainedPluginContent(ctx context.Context, stores *persistence.Bundle, digest qimu.Digest) (bool, error) {
	installations, err := stores.Installations.List(ctx)
	if err != nil {
		return false, err
	}
	for _, snapshot := range installations {
		record := snapshot.Installation.Snapshot()
		if record.Selected.String() == digest.String() || record.Staged != nil && record.Staged.String() == digest.String() {
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
		if dependency.Digest.String() == digest.String() {
			return true, nil
		}
	}
	executions, err := stores.Qimu.ExecutionDependencies(ctx)
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

func buildFramework(ctx context.Context, cfg assemblyConfig, lifetime *runtimeLifetime, packages *pluginpackage.Releases, callbacks *qimumcp.Callbacks) (frameworkComposition, error) {
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
	profile, err := process.New(process.Config{Runner: cfg.processRunner, Store: cfg.stores.Qimu, Source: source, Configure: source.Configure, CloseTimeout: 10 * time.Second, MaxFrameBytes: sdk.DefaultMaxLineLength})
	if err != nil {
		return frameworkComposition{}, err
	}
	owner := qimu.NewLifetime(context.WithoutCancel(lifetime.context))
	host, err := qimu.NewHost(ctx, qimu.HostConfig{Admission: cfg.stores.Admission, Lifetime: owner, Epoch: cfg.stores.Qimu.Epoch(), MaxInstallations: 128, Drivers: []qimu.Driver{profile}, Installations: cfg.stores.Qimu, ExecutionProtection: cfg.stores.Qimu})
	if err != nil {
		return frameworkComposition{}, err
	}
	lifetime.toolResources = append(lifetime.toolResources, terminalStep(func(ctx context.Context) error {
		_, diagnostic := host.Close(ctx)
		// An unconfirmed resource must keep the SQLite authority and content
		// protected. Caller deadlines only stop their wait for this generation.
		_, releaseErr := owner.BeginClose(qimu.ErrClosed).WaitReleased(ctx)
		return errors.Join(diagnostic, releaseErr)
	}))
	registry, err := plugins.NewRegistry(host, cfg.stores.MCPServers, cfg.stores.Installations, packages, cfg.stores.ProviderSelections, cfg.stores.PluginCompositions)
	if err != nil {
		return frameworkComposition{}, err
	}
	browserSource, err := browseradapter.NewSource(cfg.stores.Installations, packages)
	if err != nil {
		return frameworkComposition{}, err
	}
	bridge, err := controlhost.NewBridge(controlhost.BridgeConfig{Host: host, Store: cfg.stores.Qimu, Source: browserSource, PublicConfiguration: browserSource.Configuration, MaxRealms: 64, MaxPending: 4096, MaxRequestBytes: 4 << 20})
	if err != nil {
		return frameworkComposition{}, err
	}
	if err := host.RegisterDriver(bridge); err != nil {
		return frameworkComposition{}, err
	}
	browser, err := browseradapter.New(browseradapter.Config{Bridge: bridge, Registry: registry, ControlEndpoint: flamehttp.PluginControlPath, MaximumMessageBytes: 4 << 20, ProofCapacity: 4096, PollInterval: 100 * time.Millisecond})
	if err != nil {
		return frameworkComposition{}, err
	}
	if err := restoreFramework(ctx, cfg.stores, host); err != nil {
		return frameworkComposition{}, err
	}
	recovery := &frameworkRecovery{stores: cfg.stores, process: profile, pending: true, ready: make(chan struct{})}
	if err := recovery.restore(ctx); err != nil && !errors.Is(err, qimu.ErrRecoveryRequired) {
		return frameworkComposition{}, err
	}
	if cfg.firstPartyDirectory != "" {
		if err := initializeFirstPartyPlugins(ctx, cfg.stores, host, packages, cfg.firstPartyDirectory); err != nil {
			return frameworkComposition{}, err
		}
	}
	installed, err := mcpadapter.NewInstalled(mcpadapter.InstalledConfig{
		Host: host, Driver: profile, Source: source, Registry: registry, Lifetime: owner,
		Bindings: cfg.stores.PendingPluginBindings, Ready: recovery.ready,
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
	return frameworkComposition{host: host, process: profile, registry: registry, installed: installed, browser: browser, control: control, commands: commands, callers: callers, ready: recovery.ready}, nil
}

func restoreFramework(ctx context.Context, stores *persistence.Bundle, host *qimu.Host) error {
	records, err := stores.Qimu.RecoverInstallations(ctx)
	if err != nil {
		return err
	}
	definitions := make([]*qimu.Plugin, 0, len(records))
	for _, record := range records {
		digest, err := identity.ParseDigest(record.Release.String())
		if err != nil {
			return err
		}
		release, err := stores.PluginReleases.Get(ctx, digest)
		if err != nil {
			return err
		}
		projection, err := plugins.ProjectFrameworkRelease(release)
		if err != nil {
			return err
		}
		if err := plugins.RegisterFrameworkRelease(host, projection); err != nil {
			return err
		}
		definitions = append(definitions, projection.Plugin)
	}
	if err := host.Restore(ctx, records, definitions); err != nil {
		return fmt.Errorf("plugins: restore original framework records: %w", err)
	}
	if err := stores.Qimu.ConfirmRestoredInstallations(ctx); err != nil {
		return err
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
		return errors.Join(qimu.ErrRecoveryRequired, err)
	}
	err := r.stores.Admission.Recover(ctx, func() error {
		pending, err := r.stores.PluginUses.Pending(ctx)
		if err != nil {
			return err
		}
		if len(pending) != 0 {
			return qimu.ErrRecoveryRequired
		}
		dependencies, err := r.stores.Qimu.ExecutionDependencies(ctx)
		if err != nil {
			return err
		}
		if len(dependencies) != 0 {
			return qimu.ErrRecoveryRequired
		}
		return nil
	})
	if err == nil {
		r.pending = false
		close(r.ready)
	}
	return err
}
