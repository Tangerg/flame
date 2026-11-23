package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/telemetry"

	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

type catalog interface {
	Get(context.Context, framevalue.Digest) (plugin.Release, error)
}

type packages interface {
	Project(plugin.Release) (FrameworkRelease, error)
	Materialize(context.Context, string) (Candidate, error)
	Reclaim(context.Context) error
	Prepare(context.Context, plugin.Installation, plugin.Release) error
	Realize(context.Context, plugin.Installation, plugin.Release) (Realization, error)
	ReadResource(context.Context, plugin.Installation, string) ([]byte, error)
	ReadLocale(context.Context, plugin.Installation, plugin.Locale) (map[string]string, error)
}

type Candidate interface {
	Release() plugin.Release
	Publish(context.Context) (plugin.Release, error)
	Discard() error
}

type connections interface {
	WithdrawInstallation(context.Context, []mcpserver.ID) error
	ReconcileInstallation(context.Context, []mcpserver.ID) error
}

type Coordinator struct {
	lifetime    context.Context
	framework   framehost.Host
	store       Store
	catalog     catalog
	packages    packages
	connections connections
	publish     invalidation.Publish
	transact    func(context.Context, func(context.Context) error) error
}

func New(lifetime context.Context, host framehost.Host, store Store, catalog catalog, packages packages, connections connections, transact func(context.Context, func(context.Context) error) error, publish invalidation.Publish) (*Coordinator, error) {
	if !host.Valid() {
		return nil, errors.New("plugins: framework host is required")
	}
	if lo.IsNil(lifetime) {
		return nil, errors.New("plugins: runtime lifetime is required")
	}
	for _, required := range []struct {
		name  string
		value any
	}{
		{"installation store", store}, {"release catalog", catalog}, {"packages", packages},
		{"connections", connections}, {"transaction", transact}, {"invalidations", publish},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("plugins: %s is required", required.name)
		}
	}
	return &Coordinator{lifetime: lifetime, framework: host, store: store, catalog: catalog, packages: packages, connections: connections, transact: transact, publish: publish}, nil
}

type Inspection struct {
	View         plugin.View
	Selected     plugin.Release
	Staged       *plugin.Release
	Realization  Realization
	Presentation Presentation
}

func (c *Coordinator) installationSnapshot(ctx context.Context, id identity.InstallationID) (plugin.Snapshot, error) {
	snapshot, found, err := c.store.Get(ctx, id)
	if err != nil {
		return plugin.Snapshot{}, err
	}
	if !found {
		return plugin.Snapshot{}, plugin.ErrNotFound
	}
	return snapshot, nil
}

type Presentation string

const (
	PresentationAdmitted Presentation = "admitted"
	PresentationWithheld Presentation = "withheld"
)

type Realization struct {
	Release             ReleaseState
	UnavailableBackends []mcpserver.ServerName
}

type ReleaseState string

const (
	ReleaseAvailable   ReleaseState = "available"
	ReleaseUnavailable ReleaseState = "unavailable"
)

func (c *Coordinator) List(ctx context.Context) ([]Inspection, error) {
	installations, err := c.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("plugins: list installations: %w", err)
	}
	result := make([]Inspection, 0, len(installations))
	for _, snapshot := range installations {
		inspection, err := c.inspect(ctx, snapshot)
		if err != nil {
			return nil, err
		}
		result = append(result, inspection)
	}
	return result, nil
}

func (c *Coordinator) inspect(ctx context.Context, snapshot plugin.Snapshot) (Inspection, error) {
	installation, selected := snapshot.Installation, snapshot.Selected
	view, err := installation.View(selected)
	if err != nil {
		return Inspection{}, err
	}
	inspection := Inspection{View: view, Selected: selected, Staged: snapshot.Staged}
	if inspection.Realization, err = c.packages.Realize(ctx, installation, selected); err != nil {
		return Inspection{}, fmt.Errorf("plugins: observe installation %s: %w", installation.ID(), err)
	}
	inspection.Presentation = PresentationWithheld
	if installation.Enabled() && inspection.Realization.Release == ReleaseAvailable {
		inspection.Presentation = PresentationAdmitted
	}
	return inspection, nil
}

func (c *Coordinator) named(ctx context.Context, digest framevalue.Digest) (plugin.Release, error) {
	release, err := c.catalog.Get(ctx, digest)
	if errors.Is(err, plugin.ErrNotFound) {
		return plugin.Release{}, fmt.Errorf("%w: release %s is not admitted", plugin.ErrStale, digest)
	}
	return release, err
}

func (c *Coordinator) Install(ctx context.Context, source string) (_ Inspection, err error) {
	candidate, err := c.packages.Materialize(ctx, source)
	if err != nil {
		return Inspection{}, fmt.Errorf("plugins: materialize package: %w", err)
	}
	defer func() { err = errors.Join(err, candidate.Discard()) }()
	id, err := identity.ParseInstallation(uuid.NewString())
	if err != nil {
		return Inspection{}, fmt.Errorf("plugins: allocate installation identity: %w", err)
	}
	installation, err := plugin.New(id, source, candidate.Release())
	if err != nil {
		return Inspection{}, fmt.Errorf("plugins: admit installation: %w", err)
	}
	published, err := candidate.Publish(ctx)
	if err != nil {
		return Inspection{}, fmt.Errorf("plugins: publish release: %w", err)
	}
	projection, err := c.packages.Project(published)
	if err != nil {
		return Inspection{}, err
	}
	if err := RegisterFrameworkRelease(c.framework, projection); err != nil {
		return Inspection{}, err
	}
	configuration, err := EncodeFrameworkConfiguration(installation.Snapshot())
	if err != nil {
		return Inspection{}, err
	}
	_, err = c.framework.Install(ctx, framehost.InstallationConfig{ID: id.Host(), Plugin: projection.Plugin, Configuration: configuration, Enabled: installation.Enabled()})
	if err != nil {
		return Inspection{}, FrameworkError(err)
	}
	committed := plugin.Snapshot{Installation: installation, Selected: published}
	unreclaimed := c.Reclaim(ctx)
	c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins})
	realizationCtx, finishRealization := c.realizationContext(ctx)
	defer finishRealization()
	inspection, err := c.inspect(realizationCtx, committed)
	if err = errors.Join(err, unreclaimed); err != nil {
		return Inspection{}, fmt.Errorf("plugins: installation %s changed: %w", id, err)
	}
	return inspection, nil
}

func (c *Coordinator) Stage(ctx context.Context, id identity.InstallationID, source string) (_ Inspection, err error) {
	candidate, err := c.packages.Materialize(ctx, source)
	if err != nil {
		return Inspection{}, fmt.Errorf("plugins: materialize package: %w", err)
	}
	defer func() { err = errors.Join(err, candidate.Discard()) }()
	published, err := candidate.Publish(ctx)
	if err != nil {
		return Inspection{}, err
	}
	err = c.transact(ctx, func(admitted context.Context) error {
		snapshot, err := c.installationSnapshot(admitted, id)
		if err != nil {
			return err
		}
		if _, err := snapshot.Installation.Stage(snapshot.Selected, published); err != nil {
			return err
		}
		return c.store.Stage(admitted, id, published.Digest())
	})
	if err != nil {
		return Inspection{}, err
	}
	c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins})
	snapshot, err := c.installationSnapshot(ctx, id)
	if err != nil {
		return Inspection{}, err
	}
	inspection, err := c.inspect(ctx, snapshot)
	return inspection, errors.Join(err, c.Reclaim(ctx))
}

func (c *Coordinator) Reclaim(ctx context.Context) error {
	if err := c.packages.Reclaim(ctx); err != nil {
		return fmt.Errorf("plugins: reclaim releases: %w", err)
	}
	return nil
}

func (c *Coordinator) Select(ctx context.Context, id identity.InstallationID, digest framevalue.Digest) (Inspection, error) {
	return c.change(ctx, id, func(ctx context.Context, i plugin.Installation, selected plugin.Release) (plugin.Installation, error) {
		staged, err := c.named(ctx, digest)
		if err != nil {
			return plugin.Installation{}, err
		}
		return i.Select(selected, staged)
	})
}

func (c *Coordinator) Enable(ctx context.Context, id identity.InstallationID) (Inspection, error) {
	return c.change(ctx, id, func(_ context.Context, i plugin.Installation, selected plugin.Release) (plugin.Installation, error) {
		return i.Enable(selected)
	})
}

func (c *Coordinator) Disable(ctx context.Context, id identity.InstallationID) (Inspection, error) {
	return c.change(ctx, id, func(_ context.Context, i plugin.Installation, _ plugin.Release) (plugin.Installation, error) {
		return i.Disable(), nil
	})
}

func (c *Coordinator) Configure(ctx context.Context, id identity.InstallationID, digest framevalue.Digest, configuration plugin.Configuration) (Inspection, error) {
	return c.change(ctx, id, func(ctx context.Context, i plugin.Installation, _ plugin.Release) (plugin.Installation, error) {
		release, err := c.named(ctx, digest)
		if err != nil {
			return plugin.Installation{}, err
		}
		return i.Configure(release, configuration)
	})
}

func (c *Coordinator) currentFrameworkInstallation(snapshot plugin.Snapshot) (framehost.InstallationRecord, error) {
	expected, found := c.framework.Installation(snapshot.Installation.ID().Host())
	if !found {
		return framehost.InstallationRecord{}, plugin.ErrNotFound
	}
	configuration, err := EncodeFrameworkConfiguration(snapshot.Installation.Snapshot())
	if err != nil {
		return framehost.InstallationRecord{}, err
	}
	if expected.Release != snapshot.Selected.Digest() || expected.Enabled != snapshot.Installation.Enabled() || !bytes.Equal(expected.Configuration, configuration) {
		return framehost.InstallationRecord{}, plugin.ErrStale
	}
	return expected, nil
}

func changedServerIDs(installation plugin.Installation, before plugin.Snapshot, after plugin.Release) ([]mcpserver.ID, error) {
	changed, err := installation.ChangedServers(before.Installation, before.Selected, after)
	if err != nil {
		return nil, err
	}
	ids := make([]mcpserver.ID, 0, len(changed))
	for _, name := range changed {
		server, err := installation.ServerID(name)
		if err != nil {
			return nil, err
		}
		ids = append(ids, server)
	}
	return ids, nil
}

func (c *Coordinator) change(ctx context.Context, id identity.InstallationID, transition func(context.Context, plugin.Installation, plugin.Release) (plugin.Installation, error)) (Inspection, error) {
	snapshot, err := c.installationSnapshot(ctx, id)
	if err != nil {
		return Inspection{}, err
	}
	expected, err := c.currentFrameworkInstallation(snapshot)
	if err != nil {
		return Inspection{}, err
	}
	installation, err := transition(ctx, snapshot.Installation, snapshot.Selected)
	if err != nil {
		return Inspection{}, err
	}
	after := snapshot.Selected
	if installation.Selected() != after.Digest() {
		after, err = c.catalog.Get(ctx, installation.Selected())
		if err != nil {
			return Inspection{}, err
		}
	}
	projection, err := c.packages.Project(after)
	if err != nil {
		return Inspection{}, err
	}
	if err := RegisterFrameworkRelease(c.framework, projection); err != nil {
		return Inspection{}, err
	}
	configuration, err := EncodeFrameworkConfiguration(installation.Snapshot())
	if err != nil {
		return Inspection{}, err
	}
	if err := c.framework.Update(ctx, expected, framehost.InstallationConfig{ID: expected.ID, Plugin: projection.Plugin, Configuration: configuration, Enabled: installation.Enabled()}); err != nil {
		return Inspection{}, FrameworkError(err)
	}
	reconcile, err := changedServerIDs(installation, snapshot, after)
	if err != nil {
		return Inspection{}, err
	}
	withdrawErr := c.connections.WithdrawInstallation(ctx, reconcile)
	c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins}, invalidation.Notice{Resource: invalidation.Skills})
	committed, err := c.installationSnapshot(ctx, id)
	if err != nil {
		return Inspection{}, errors.Join(err, withdrawErr)
	}
	realizationCtx, finish := c.realizationContext(ctx)
	defer finish()
	if len(reconcile) > 0 {
		if err := c.packages.Prepare(realizationCtx, committed.Installation, committed.Selected); err != nil {
			slog.WarnContext(ctx, "committed plugin preparation failed", telemetry.InstallationID(id), telemetry.Error(err))
		}
		if err := c.connections.ReconcileInstallation(realizationCtx, reconcile); err != nil {
			slog.WarnContext(ctx, "committed plugin reconciliation failed", telemetry.InstallationID(id), telemetry.Error(err))
		}
	}
	inspection, err := c.inspect(realizationCtx, committed)
	return inspection, errors.Join(err, withdrawErr, c.Reclaim(realizationCtx))
}

func (c *Coordinator) Uninstall(ctx context.Context, id identity.InstallationID) error {
	snapshot, err := c.installationSnapshot(ctx, id)
	if err != nil {
		return err
	}
	servers, err := snapshot.Installation.ServerIDs(snapshot.Selected)
	if err != nil {
		return err
	}
	if err := c.framework.Uninstall(ctx, id.Host()); err != nil {
		return FrameworkError(err)
	}
	withdrawErr := c.connections.WithdrawInstallation(ctx, servers)
	c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins}, invalidation.Notice{Resource: invalidation.Skills})
	realizationCtx, finish := c.realizationContext(ctx)
	defer finish()
	if err := c.connections.ReconcileInstallation(realizationCtx, servers); err != nil {
		slog.WarnContext(ctx, "removed plugin reconciliation failed", telemetry.InstallationID(id), telemetry.Error(err))
	}
	return errors.Join(withdrawErr, c.Reclaim(realizationCtx))
}

// A committed change must survive request cancellation; shutdown retires its I/O
// through the Runtime's cancellation root.
func (c *Coordinator) realizationContext(request context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(context.WithoutCancel(request))
	stop := context.AfterFunc(c.lifetime, func() { cancel(context.Cause(c.lifetime)) })
	if cause := context.Cause(c.lifetime); cause != nil {
		cancel(cause)
	}
	return ctx, func() {
		stop()
		cancel(nil)
	}
}
