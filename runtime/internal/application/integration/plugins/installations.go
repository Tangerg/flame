package plugins

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/google/uuid"
)

// Catalog is the admitted release catalog, the only owner of release content.
// Installations name releases by digest and read them here when needed.
type Catalog interface {
	Get(context.Context, fingerprint.Digest) (plugin.Release, error)
}

// Packages owns release bytes: it admits a package as a candidate, publishes
// and reclaims releases, prepares backends and observes realization.
type Packages interface {
	Materialize(context.Context, string) (Candidate, error)
	Reclaim(context.Context, []fingerprint.Digest) error
	Prepare(context.Context, *plugin.Installation, plugin.Release) error
	Realize(context.Context, *plugin.Installation, plugin.Release) (Realization, error)
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
type Connections interface {
	WithdrawInstallation([]mcpserver.ID) error
	ReconcileInstallation(context.Context, []mcpserver.ID) error
}

// Dependencies is the single serialization point for installation changes. It
// runs each change atomically with execution admission, so capacity, quiescence,
// the durable write, the withdrawal of superseded tools and release reclamation
// are decided against one consistent state. Each step receives the canonical
// releases execution holds: those of live sessions and pending checkpoints.
type Dependencies interface {
	ChangeInstallation(context.Context, resourceid.InstallationID, ChangeAdmission, func(held []plugin.Dependency) error) error
	UnderAdmission(context.Context, func(held []plugin.Dependency) error) error
}

type ChangeAdmission uint8

const (
	RequireQuiescent ChangeAdmission = iota
	AllowInUse
)

type Coordinator struct {
	lifetime     context.Context
	store        Store
	catalog      Catalog
	packages     Packages
	connections  Connections
	dependencies Dependencies
	publish      invalidation.Publish
}

func New(lifetime context.Context, store Store, catalog Catalog, packages Packages, connections Connections, dependencies Dependencies, publish invalidation.Publish) (*Coordinator, error) {
	if lifetime == nil {
		return nil, errors.New("plugins: runtime lifetime is required")
	}
	if store == nil || catalog == nil || packages == nil || connections == nil || dependencies == nil {
		return nil, errors.New("plugins: installation dependencies are required")
	}
	return &Coordinator{lifetime: lifetime, store: store, catalog: catalog, packages: packages, connections: connections, dependencies: dependencies, publish: publish}, nil
}

// Inspection projects an installation beside the catalog releases it names.
type Inspection struct {
	Record       plugin.Record
	Selected     plugin.Release
	Staged       *plugin.Release
	Realization  Realization
	Presentation Presentation
}

// Presentation is whether the selected release's declarative presentation
// contributions, such as themes, may be shown now. Runtime alone decides it, so
// a client never rebuilds activation from desired state and observed bytes.
type Presentation string

const (
	PresentationAdmitted Presentation = "admitted"
	PresentationWithheld Presentation = "withheld"
)

// Realization is what the selected release's bytes and backends show now: the
// release state and every declared server as an MCP source with its
// availability. The package owner builds it once per read and it is never
// retained, so a repaired or lost backend is reflected by the next read rather
// than by a remembered change outcome. The installation and MCP projections
// both read this one value.
type Realization struct {
	Release ReleaseState
	Sources []mcpapp.Source
}

// UnavailableBackends names the declared servers of an available release whose
// backend cannot be realized now.
func (r Realization) UnavailableBackends() []mcpserver.ServerName {
	var names []mcpserver.ServerName
	for _, source := range r.Sources {
		if source.Availability == mcpapp.SourceUnavailableBackend {
			names = append(names, source.Server.Name)
		}
	}
	return names
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
	for _, installation := range installations {
		inspection, err := c.inspect(ctx, installation)
		if err != nil {
			return nil, err
		}
		result = append(result, inspection)
	}
	return result, nil
}

func (c *Coordinator) inspect(ctx context.Context, installation *plugin.Installation) (Inspection, error) {
	record := installation.Snapshot()
	selected, err := c.catalog.Get(ctx, record.Selected)
	if err != nil {
		return Inspection{}, fmt.Errorf("plugins: read release %s: %w", record.Selected, err)
	}
	inspection := Inspection{Record: record, Selected: selected}
	if digest, staged := installation.Staged(); staged {
		release, err := c.catalog.Get(ctx, digest)
		if err != nil {
			return Inspection{}, fmt.Errorf("plugins: read release %s: %w", digest, err)
		}
		inspection.Staged = &release
	}
	if inspection.Realization, err = c.packages.Realize(ctx, installation, selected); err != nil {
		return Inspection{}, fmt.Errorf("plugins: observe installation %s: %w", record.ID, err)
	}
	inspection.Presentation = PresentationWithheld
	if installation.Active() && inspection.Realization.Release == ReleaseAvailable {
		inspection.Presentation = PresentationAdmitted
	}
	return inspection, nil
}

// named reads a release a client named by digest. A digest the catalog has
// never admitted cannot be the installation's release, so it is stale.
func (c *Coordinator) named(ctx context.Context, digest fingerprint.Digest) (plugin.Release, error) {
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
	id, err := resourceid.ParseInstallation(uuid.NewString())
	if err != nil {
		return Inspection{}, fmt.Errorf("plugins: allocate installation identity: %w", err)
	}
	installation, err := plugin.New(id, source, candidate.Release())
	if err != nil {
		return Inspection{}, fmt.Errorf("plugins: admit installation: %w", err)
	}
	unreclaimed, err := c.commit(ctx, id, AllowInUse, func() error {
		existing, err := c.store.List(ctx)
		if err != nil {
			return err
		}
		if len(existing) >= plugin.MaxInstallations {
			return fmt.Errorf("%w: installation capacity", plugin.ErrInvalid)
		}
		if _, err := candidate.Publish(ctx); err != nil {
			return fmt.Errorf("plugins: publish release: %w", err)
		}
		return c.store.Save(ctx, installation)
	})
	if err != nil {
		return Inspection{}, errors.Join(err, unreclaimed)
	}
	c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins})
	inspection, err := c.inspect(ctx, installation)
	if err = errors.Join(err, unreclaimed); err != nil {
		return Inspection{}, fmt.Errorf("plugins: installation %s changed: %w", id, err)
	}
	return inspection, nil
}

func (c *Coordinator) Stage(ctx context.Context, id resourceid.InstallationID, source string) (_ Inspection, err error) {
	candidate, err := c.packages.Materialize(ctx, source)
	if err != nil {
		return Inspection{}, fmt.Errorf("plugins: materialize package: %w", err)
	}
	defer func() { err = errors.Join(err, candidate.Discard()) }()
	return c.change(ctx, id, AllowInUse, func(ctx context.Context, i *plugin.Installation, selected plugin.Release) error {
		if err := i.Stage(selected, candidate.Release()); err != nil {
			return err
		}
		if _, err := candidate.Publish(ctx); err != nil {
			return fmt.Errorf("plugins: publish release: %w", err)
		}
		return nil
	})
}

// Reclaim removes every release nothing references, such as one whose change
// failed after publication when the Runtime stopped before reclaiming it.
func (c *Coordinator) Reclaim(ctx context.Context) error {
	return c.dependencies.UnderAdmission(ctx, func(held []plugin.Dependency) error {
		return c.reclaim(ctx, held)
	})
}

// commit runs one change at the admission point and then, in the same critical
// section and whether or not the change committed, reclaims every release that
// nothing references: a refused stage or install leaves no release behind. A
// release that cannot be reclaimed stays admitted, and so owned for the next
// commit; that failure is returned apart from the change's, because the change
// itself stands.
func (c *Coordinator) commit(ctx context.Context, id resourceid.InstallationID, admission ChangeAdmission, change func() error) (unreclaimed, err error) {
	err = c.dependencies.ChangeInstallation(ctx, id, admission, func(held []plugin.Dependency) error {
		changeErr := change()
		unreclaimed = c.reclaim(ctx, held)
		return changeErr
	})
	return unreclaimed, err
}

func (c *Coordinator) reclaim(ctx context.Context, held []plugin.Dependency) error {
	installations, err := c.store.List(ctx)
	if err != nil {
		return fmt.Errorf("plugins: reclaim releases: %w", err)
	}
	if err := c.packages.Reclaim(ctx, plugin.RetainedReleases(installations, held)); err != nil {
		return fmt.Errorf("plugins: reclaim releases: %w", err)
	}
	return nil
}

func (c *Coordinator) Select(ctx context.Context, id resourceid.InstallationID, digest fingerprint.Digest) (Inspection, error) {
	return c.change(ctx, id, RequireQuiescent, func(ctx context.Context, i *plugin.Installation, selected plugin.Release) error {
		staged, err := c.named(ctx, digest)
		if err != nil {
			return err
		}
		return i.Select(selected, staged)
	})
}
func (c *Coordinator) Approve(ctx context.Context, id resourceid.InstallationID, digest fingerprint.Digest) (Inspection, error) {
	return c.change(ctx, id, RequireQuiescent, func(ctx context.Context, i *plugin.Installation, _ plugin.Release) error {
		release, err := c.named(ctx, digest)
		if err != nil {
			return err
		}
		return i.Approve(release)
	})
}
func (c *Coordinator) Enable(ctx context.Context, id resourceid.InstallationID) (Inspection, error) {
	return c.change(ctx, id, RequireQuiescent, func(_ context.Context, i *plugin.Installation, selected plugin.Release) error {
		return i.Enable(selected)
	})
}

// Disable withdraws authority, so like Revoke it is available while in use.
func (c *Coordinator) Disable(ctx context.Context, id resourceid.InstallationID) (Inspection, error) {
	return c.change(ctx, id, AllowInUse, func(_ context.Context, i *plugin.Installation, _ plugin.Release) error {
		i.Disable()
		return nil
	})
}
func (c *Coordinator) Revoke(ctx context.Context, id resourceid.InstallationID) (Inspection, error) {
	return c.change(ctx, id, AllowInUse, func(_ context.Context, i *plugin.Installation, _ plugin.Release) error {
		i.Revoke()
		return nil
	})
}
func (c *Coordinator) Configure(ctx context.Context, id resourceid.InstallationID, digest fingerprint.Digest, configuration plugin.Configuration) (Inspection, error) {
	return c.change(ctx, id, RequireQuiescent, func(ctx context.Context, i *plugin.Installation, _ plugin.Release) error {
		release, err := c.named(ctx, digest)
		if err != nil {
			return err
		}
		return i.Configure(release, configuration)
	})
}
func (c *Coordinator) change(ctx context.Context, id resourceid.InstallationID, admission ChangeAdmission, transition func(context.Context, *plugin.Installation, plugin.Release) error) (Inspection, error) {
	var reconcile []mcpserver.ID
	var committed *plugin.Installation
	var release plugin.Release
	unreclaimed, err := c.commit(ctx, id, admission, func() error {
		installation, err := c.store.Get(ctx, id)
		if err != nil {
			return err
		}
		previous, err := plugin.Restore(installation.Snapshot())
		if err != nil {
			return err
		}
		before, err := c.catalog.Get(ctx, installation.Selected())
		if err != nil {
			return fmt.Errorf("plugins: read release %s: %w", installation.Selected(), err)
		}
		if err := transition(ctx, installation, before); err != nil {
			return err
		}
		after := before
		if installation.Selected() != before.Digest() {
			if after, err = c.catalog.Get(ctx, installation.Selected()); err != nil {
				return fmt.Errorf("plugins: read release %s: %w", installation.Selected(), err)
			}
		}
		changed, err := installation.ChangedServers(previous, before, after)
		if err != nil {
			return err
		}
		for _, local := range changed {
			server, err := installation.ServerID(local)
			if err != nil {
				return err
			}
			reconcile = append(reconcile, server)
		}
		if err := c.store.Save(ctx, installation); err != nil {
			return err
		}
		committed, release = installation, after
		// The live tool catalog follows the commit inside the same critical
		// section: a Run assembled after it cannot see the superseded tools.
		return c.connections.WithdrawInstallation(reconcile)
	})
	if err != nil {
		return Inspection{}, errors.Join(err, unreclaimed)
	}
	realizationCtx, finishRealization := c.realizationContext(ctx)
	defer finishRealization()
	if len(reconcile) > 0 {
		// Realization outcomes have live owners and are observed, not reported:
		// an unprepared backend reads as unavailable in every inspection, and
		// each connection's failure is its supervisor's status. These records
		// keep only the operational cause for the trace.
		if err := c.packages.Prepare(realizationCtx, committed, release); err != nil {
			slog.WarnContext(ctx, "committed plugin preparation failed", "installation", id, "error", err)
		}
		if err := c.connections.ReconcileInstallation(realizationCtx, reconcile); err != nil {
			slog.WarnContext(ctx, "committed plugin reconciliation failed", "installation", id, "error", err)
		}
	}
	c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins}, invalidation.Notice{Resource: invalidation.Skills})
	// The result reports observed realization or why it could not be observed;
	// either way the durable change above stands and has been published.
	inspection, err := c.inspect(realizationCtx, committed)
	if err = errors.Join(err, unreclaimed); err != nil {
		return Inspection{}, fmt.Errorf("plugins: installation %s changed: %w", id, err)
	}
	return inspection, nil
}
func (c *Coordinator) Uninstall(ctx context.Context, id resourceid.InstallationID) error {
	var servers []mcpserver.ID
	// Removal withdraws every artifact a waiting operation needs to resume, so
	// it requires quiescence; Revoke stays available to withdraw authority
	// from work that is still in use.
	unreclaimed, err := c.commit(ctx, id, RequireQuiescent, func() error {
		installation, err := c.store.Get(ctx, id)
		if err != nil {
			return err
		}
		release, err := c.catalog.Get(ctx, installation.Selected())
		if err != nil {
			return fmt.Errorf("plugins: read release %s: %w", installation.Selected(), err)
		}
		servers, err = installation.ServerIDs(release)
		if err != nil {
			return err
		}
		if err := c.store.Remove(ctx, id); err != nil {
			return err
		}
		return c.connections.WithdrawInstallation(servers)
	})
	if err != nil {
		return errors.Join(err, unreclaimed)
	}
	// Removal has withdrawn admission durably and the live tools with it.
	// Connections retains and joins teardown independently of this request,
	// including during shutdown. Reconciliation retires exposure and status.
	realizationCtx, finishRealization := c.realizationContext(ctx)
	defer finishRealization()
	if err := c.connections.ReconcileInstallation(realizationCtx, servers); err != nil {
		slog.WarnContext(ctx, "removed plugin reconciliation failed", "installation", id, "error", err)
	}
	c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins}, invalidation.Notice{Resource: invalidation.Skills})
	if unreclaimed != nil {
		return fmt.Errorf("plugins: installation %s removed: %w", id, unreclaimed)
	}
	return nil
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
