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

func (c *Coordinator) Reclaim(ctx context.Context) error {
	if err := c.packages.Reclaim(ctx); err != nil {
		return fmt.Errorf("plugins: reclaim releases: %w", err)
	}
	return nil
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

func (c *Coordinator) Configure(ctx context.Context, id identity.InstallationID, digest framevalue.Digest, configuration plugin.Configuration) (Inspection, error) {
	snapshot, err := c.installationSnapshot(ctx, id)
	if err != nil {
		return Inspection{}, err
	}
	if snapshot.Selected.Digest() != digest {
		return Inspection{}, fmt.Errorf("%w: release %s is not selected", plugin.ErrStale, digest)
	}
	installation, err := snapshot.Installation.Configure(snapshot.Selected, configuration)
	if err != nil {
		return Inspection{}, err
	}
	reconcile, err := c.commitInstallationChange(ctx, snapshot, installation, snapshot.Selected)
	if err != nil {
		return Inspection{}, err
	}
	ownerCtx, finish := c.realizationContext(ctx)
	defer finish()
	committed, err := c.realizeInstallationChange(ownerCtx, id, reconcile)
	var inspection Inspection
	if err == nil {
		inspection, err = c.inspect(ownerCtx, committed)
	}
	return inspection, errors.Join(err, c.Reclaim(ownerCtx))
}

func (c *Coordinator) commitInstallationChange(ctx context.Context, before plugin.Snapshot, installation plugin.Installation, after plugin.Release) ([]mcpserver.ID, error) {
	expected, err := c.currentFrameworkInstallation(before)
	if err != nil {
		return nil, err
	}
	reconcile, err := changedServerIDs(installation, before, after)
	if err != nil {
		return nil, err
	}
	projection, err := c.packages.Project(after)
	if err != nil {
		return nil, err
	}
	if err := RegisterFrameworkRelease(c.framework, projection); err != nil {
		return nil, err
	}
	configuration, err := EncodeFrameworkConfiguration(installation.Snapshot())
	if err != nil {
		return nil, err
	}
	err = c.framework.Update(ctx, expected, framehost.InstallationConfig{ID: expected.ID, Plugin: projection.Plugin, Configuration: configuration, Enabled: installation.Enabled()})
	return reconcile, FrameworkError(err)
}

func (c *Coordinator) reconcileCommittedInstallation(ctx context.Context, id identity.InstallationID, servers []mcpserver.ID, commitErr error) {
	ownerCtx, finish := c.realizationContext(ctx)
	defer finish()
	_, realizationErr := c.realizeInstallationChange(ownerCtx, id, servers)
	if err := errors.Join(commitErr, realizationErr, c.Reclaim(ownerCtx)); err != nil {
		slog.WarnContext(ownerCtx, "committed plugin update could not fully reconcile", telemetry.InstallationID(id), telemetry.Error(err))
	}
}

func (c *Coordinator) realizeInstallationChange(ctx context.Context, id identity.InstallationID, reconcile []mcpserver.ID) (plugin.Snapshot, error) {
	withdrawErr := c.connections.WithdrawInstallation(ctx, reconcile)
	c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins}, invalidation.Notice{Resource: invalidation.Skills})
	committed, err := c.installationSnapshot(ctx, id)
	if err != nil {
		return plugin.Snapshot{}, errors.Join(err, withdrawErr)
	}
	if len(reconcile) > 0 {
		if err := c.packages.Prepare(ctx, committed.Installation, committed.Selected); err != nil {
			slog.WarnContext(ctx, "committed plugin preparation failed", telemetry.InstallationID(id), telemetry.Error(err))
		}
		if err := c.connections.ReconcileInstallation(ctx, reconcile); err != nil {
			slog.WarnContext(ctx, "committed plugin reconciliation failed", telemetry.InstallationID(id), telemetry.Error(err))
		}
	}
	return committed, withdrawErr
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
