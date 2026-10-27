package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

// Catalog is the admitted release catalog, the only owner of release content.
// Installations name releases by digest and read them here when needed.
type catalog interface {
	Get(context.Context, identity.Digest) (plugin.Release, error)
}

// Packages owns release bytes: it admits a package as a candidate, publishes
// and reclaims releases, prepares backends and observes realization.
type packages interface {
	Materialize(context.Context, string) (Candidate, error)
	Reclaim(context.Context) error
	Prepare(context.Context, *plugin.Installation, plugin.Release) error
	Realize(context.Context, *plugin.Installation, plugin.Release) (Realization, error)
	ReadResource(context.Context, *plugin.Installation, string) ([]byte, error)
}

// Candidate is one package's admitted bytes awaiting publication. Only an
// installation change publishes it, at the admission point, so a release
// becomes visible only where it can also be reclaimed. Discard removes bytes
// that were never published and is always called once.
type Candidate interface {
	Release() plugin.Release
	Publish(context.Context) (plugin.Release, error)
	Discard() error
}

// Connections realizes installation sources in the MCP supervisor.
// WithdrawInstallation runs inside the change's critical section and must not
// block; ReconcileInstallation dials after it.
type connections interface {
	WithdrawInstallation([]mcpserver.ID) error
	ReconcileInstallation(context.Context, []mcpserver.ID) error
}

type Coordinator struct {
	lifetime    context.Context
	framework   *qimu.Host
	store       Store
	catalog     catalog
	packages    packages
	connections connections
	publish     invalidation.Publish
}

func New(lifetime context.Context, host *qimu.Host, store Store, catalog catalog, packages packages, connections connections, publish invalidation.Publish) (*Coordinator, error) {
	if lo.IsNil(lifetime) {
		return nil, errors.New("plugins: runtime lifetime is required")
	}
	for _, required := range []struct {
		name  string
		value any
	}{
		{"framework host", host},
		{"installation store", store}, {"release catalog", catalog}, {"packages", packages},
		{"connections", connections},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("plugins: %s is required", required.name)
		}
	}
	return &Coordinator{lifetime: lifetime, framework: host, store: store, catalog: catalog, packages: packages, connections: connections, publish: publish}, nil
}

// Inspection projects an installation beside the catalog releases it names.
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

// Presentation is whether the selected release's declarative presentation
// contributions, such as themes, may be shown now. Runtime alone decides it, so
// a client never rebuilds activation from desired state and observed bytes.
type Presentation string

const (
	PresentationAdmitted Presentation = "admitted"
	PresentationWithheld Presentation = "withheld"
)

// Realization observes release bytes and backend availability. It is not retained,
// so loss or repair is reflected by the next inspection. An unavailable release
// has no backend observations. Desired definitions and live connections have
// independent owners.
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

// named reads a release a client named by digest. A digest the catalog has
// never admitted cannot be the installation's release, so it is stale.
func (c *Coordinator) named(ctx context.Context, digest identity.Digest) (plugin.Release, error) {
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
	projection, err := ProjectFrameworkRelease(published)
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
	_, err = c.framework.Install(ctx, qimu.InstallationConfig{ID: qimu.InstallationID(id.String()), Plugin: projection.Plugin, Configuration: configuration, Enabled: installation.Enabled()})
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
	err = c.framework.Admission().Admit(ctx, func(admitted context.Context) error {
		snapshot, err := c.installationSnapshot(admitted, id)
		if err != nil {
			return err
		}
		if err := snapshot.Installation.Stage(snapshot.Selected, published); err != nil {
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

// Reclaim removes every release nothing references, such as one whose change
// failed after publication when the Runtime stopped before reclaiming it.
func (c *Coordinator) Reclaim(ctx context.Context) error {
	if err := c.packages.Reclaim(ctx); err != nil {
		return fmt.Errorf("plugins: reclaim releases: %w", err)
	}
	return nil
}

func (c *Coordinator) Select(ctx context.Context, id identity.InstallationID, digest identity.Digest) (Inspection, error) {
	return c.change(ctx, id, func(ctx context.Context, i *plugin.Installation, selected plugin.Release) error {
		staged, err := c.named(ctx, digest)
		if err != nil {
			return err
		}
		return i.Select(selected, staged)
	})
}

// Restoring availability of the same selected code does not replace a held
// execution dependency. Selection and configuration still require quiescence.
func (c *Coordinator) Enable(ctx context.Context, id identity.InstallationID) (Inspection, error) {
	return c.change(ctx, id, func(_ context.Context, i *plugin.Installation, selected plugin.Release) error {
		return i.Enable(selected)
	})
}

// Disable withdraws future dispatch and remains available while in use.
func (c *Coordinator) Disable(ctx context.Context, id identity.InstallationID) (Inspection, error) {
	return c.change(ctx, id, func(_ context.Context, i *plugin.Installation, _ plugin.Release) error {
		i.Disable()
		return nil
	})
}
func (c *Coordinator) Configure(ctx context.Context, id identity.InstallationID, digest identity.Digest, configuration plugin.Configuration) (Inspection, error) {
	return c.change(ctx, id, func(ctx context.Context, i *plugin.Installation, _ plugin.Release) error {
		release, err := c.named(ctx, digest)
		if err != nil {
			return err
		}
		return i.Configure(release, configuration)
	})
}
func (c *Coordinator) change(ctx context.Context, id identity.InstallationID, transition func(context.Context, *plugin.Installation, plugin.Release) error) (Inspection, error) {
	snapshot, err := c.installationSnapshot(ctx, id)
	if err != nil {
		return Inspection{}, err
	}
	expected, found := c.framework.Installation(qimu.InstallationID(id.String()))
	if !found {
		return Inspection{}, plugin.ErrNotFound
	}
	configuration, err := EncodeFrameworkConfiguration(snapshot.Installation.Snapshot())
	if err != nil {
		return Inspection{}, err
	}
	if expected.Release.String() != snapshot.Selected.Digest().String() || expected.Enabled != snapshot.Installation.Enabled() || !bytes.Equal(expected.Configuration, configuration) {
		return Inspection{}, plugin.ErrStale
	}
	installation, err := plugin.Restore(snapshot.Installation.Snapshot(), snapshot.Selected, snapshot.Staged)
	if err != nil {
		return Inspection{}, err
	}
	if err := transition(ctx, installation, snapshot.Selected); err != nil {
		return Inspection{}, err
	}
	after := snapshot.Selected
	if installation.Selected() != after.Digest() {
		after, err = c.catalog.Get(ctx, installation.Selected())
		if err != nil {
			return Inspection{}, err
		}
	}
	projection, err := ProjectFrameworkRelease(after)
	if err != nil {
		return Inspection{}, err
	}
	if err := RegisterFrameworkRelease(c.framework, projection); err != nil {
		return Inspection{}, err
	}
	configuration, err = EncodeFrameworkConfiguration(installation.Snapshot())
	if err != nil {
		return Inspection{}, err
	}
	if err := c.framework.Update(ctx, expected, qimu.InstallationConfig{ID: expected.ID, Plugin: projection.Plugin, Configuration: configuration, Enabled: installation.Enabled()}); err != nil {
		return Inspection{}, FrameworkError(err)
	}
	changed, err := installation.ChangedServers(snapshot.Installation, snapshot.Selected, after)
	if err != nil {
		return Inspection{}, err
	}
	var reconcile []mcpserver.ID
	for _, name := range changed {
		server, err := installation.ServerID(name)
		if err != nil {
			return Inspection{}, err
		}
		reconcile = append(reconcile, server)
	}
	withdrawErr := c.connections.WithdrawInstallation(reconcile)
	c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins}, invalidation.Notice{Resource: invalidation.Skills})
	committed, err := c.installationSnapshot(ctx, id)
	if err != nil {
		return Inspection{}, errors.Join(err, withdrawErr)
	}
	realizationCtx, finish := c.realizationContext(ctx)
	defer finish()
	if len(reconcile) > 0 {
		if err := c.packages.Prepare(realizationCtx, committed.Installation, committed.Selected); err != nil {
			slog.WarnContext(ctx, "committed plugin preparation failed", "installation", id, "error", err)
		}
		if err := c.connections.ReconcileInstallation(realizationCtx, reconcile); err != nil {
			slog.WarnContext(ctx, "committed plugin reconciliation failed", "installation", id, "error", err)
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
	if err := c.framework.Uninstall(ctx, qimu.InstallationID(id.String())); err != nil {
		return FrameworkError(err)
	}
	withdrawErr := c.connections.WithdrawInstallation(servers)
	c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins}, invalidation.Notice{Resource: invalidation.Skills})
	realizationCtx, finish := c.realizationContext(ctx)
	defer finish()
	if err := c.connections.ReconcileInstallation(realizationCtx, servers); err != nil {
		slog.WarnContext(ctx, "removed plugin reconciliation failed", "installation", id, "error", err)
	}
	return errors.Join(withdrawErr, c.Reclaim(realizationCtx))
}

// A committed change must survive request cancellation. It remains inside the
// Endpoint invocation, which joins it before dependencies close, and follows
// the Runtime's cancellation root so that shutdown can retire its I/O.
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

func installationStates(snapshots []plugin.Snapshot) []*plugin.Installation {
	result := make([]*plugin.Installation, 0, len(snapshots))
	for _, snapshot := range snapshots {
		result = append(result, snapshot.Installation)
	}
	return result
}
