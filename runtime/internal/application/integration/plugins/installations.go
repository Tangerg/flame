package plugins

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/google/uuid"
)

type Packages interface {
	Materialize(context.Context, string) (plugin.Release, error)
	Prepare(context.Context, *plugin.Installation) ([]plugin.Diagnostic, error)
	Servers(context.Context, *plugin.Installation) (Backends, error)
}

type Connections interface {
	ReconcileInstallation(context.Context, []mcpserver.ServerName) error
}
type Dependencies interface {
	ChangeInstallation(context.Context, string, ChangeAdmission, func() error) error
}

type ChangeAdmission uint8

const (
	RequireQuiescent ChangeAdmission = iota
	AllowInUse
)

type Coordinator struct {
	lifetime     context.Context
	mu           sync.Mutex
	store        Store
	packages     Packages
	connections  Connections
	dependencies Dependencies
	publish      invalidation.Publish
}

func New(lifetime context.Context, store Store, packages Packages, connections Connections, dependencies Dependencies, publish invalidation.Publish) (*Coordinator, error) {
	if lifetime == nil {
		return nil, errors.New("plugins: runtime lifetime is required")
	}
	if store == nil || packages == nil || connections == nil || dependencies == nil {
		return nil, errors.New("plugins: installation dependencies are required")
	}
	return &Coordinator{lifetime: lifetime, store: store, packages: packages, connections: connections, dependencies: dependencies, publish: publish}, nil
}

type Removal struct {
	Availability []plugin.Diagnostic
}

type Inspection struct {
	Record       plugin.Record
	Availability []plugin.Diagnostic
}

func (c *Coordinator) List(ctx context.Context) ([]Inspection, error) {
	installations, err := c.store.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Inspection, 0, len(installations))
	for _, i := range installations {
		inspection := Inspection{Record: i.Snapshot(), Availability: []plugin.Diagnostic{}}
		backends, err := c.packages.Servers(ctx, i)
		if context.Cause(ctx) != nil {
			return nil, context.Cause(ctx)
		}
		if err != nil {
			if !errors.Is(err, plugin.ErrUnavailable) {
				return nil, err
			}
			inspection.Availability = append(inspection.Availability, plugin.Diagnostic{Component: "release", Code: "unavailable_release"})
		} else {
			inspection.Availability = append(inspection.Availability, backends.Availability...)
		}
		result = append(result, inspection)
	}
	return result, nil
}
func (c *Coordinator) Install(ctx context.Context, source string) (inspection Inspection, err error) {
	release, err := c.packages.Materialize(ctx, source)
	if err != nil {
		return Inspection{}, err
	}
	c.mu.Lock()
	defer func() {
		c.mu.Unlock()
		if inspection.Record.ID != "" {
			c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins})
		}
	}()
	existing, err := c.store.List(ctx)
	if err != nil {
		return Inspection{}, err
	}
	if len(existing) >= plugin.MaxInstallations {
		return Inspection{}, fmt.Errorf("%w: installation capacity", plugin.ErrInvalid)
	}
	installation, err := plugin.New(uuid.NewString(), source, release)
	if err != nil {
		return Inspection{}, err
	}
	if err = c.store.Save(ctx, installation); err != nil {
		return Inspection{}, err
	}
	return Inspection{Record: installation.Snapshot()}, nil
}

func (c *Coordinator) Stage(ctx context.Context, id, source string) (Inspection, error) {
	release, err := c.packages.Materialize(ctx, source)
	if err != nil {
		return Inspection{}, err
	}
	return c.change(ctx, id, AllowInUse, func(i *plugin.Installation) error { return i.Stage(release) })
}
func (c *Coordinator) Select(ctx context.Context, id, digest string) (Inspection, error) {
	return c.change(ctx, id, RequireQuiescent, func(i *plugin.Installation) error { return i.Select(digest) })
}
func (c *Coordinator) Approve(ctx context.Context, id, digest string, grants []plugin.RequestGrant) (Inspection, error) {
	return c.change(ctx, id, RequireQuiescent, func(i *plugin.Installation) error { return i.Approve(digest, grants) })
}
func (c *Coordinator) Enable(ctx context.Context, id string, enabled bool) (Inspection, error) {
	admission := AllowInUse
	if enabled {
		admission = RequireQuiescent
	}
	return c.change(ctx, id, admission, func(i *plugin.Installation) error { return i.Enable(enabled) })
}
func (c *Coordinator) Revoke(ctx context.Context, id string) (Inspection, error) {
	return c.change(ctx, id, AllowInUse, func(i *plugin.Installation) error { i.Revoke(); return nil })
}
func (c *Coordinator) Configure(ctx context.Context, id string, configuration plugin.Configuration) (Inspection, error) {
	return c.change(ctx, id, RequireQuiescent, func(i *plugin.Installation) error { return i.Configure(configuration) })
}
func (c *Coordinator) change(ctx context.Context, id string, admission ChangeAdmission, transition func(*plugin.Installation) error) (inspection Inspection, err error) {
	var reconcile []mcpserver.ServerName
	var committed *plugin.Installation
	c.mu.Lock()
	err = c.dependencies.ChangeInstallation(ctx, id, admission, func() error {
		installation, err := c.store.Get(ctx, id)
		if err != nil {
			return err
		}
		previous, err := plugin.Restore(installation.Snapshot())
		if err != nil {
			return err
		}
		if err := transition(installation); err != nil {
			return err
		}
		for _, local := range installation.ChangedServers(previous) {
			name, err := mcpserver.InstallationServer(id, local)
			if err != nil {
				return err
			}
			reconcile = append(reconcile, name)
		}
		if err := c.store.Save(ctx, installation); err != nil {
			return err
		}
		committed = installation
		inspection = Inspection{Record: installation.Snapshot()}
		return nil
	})
	c.mu.Unlock()
	if err != nil {
		return Inspection{}, err
	}
	realizationCtx, finishRealization := c.realizationContext(ctx)
	defer finishRealization()
	if len(reconcile) > 0 {
		diagnostics, err := c.packages.Prepare(realizationCtx, committed)
		inspection.Availability = append(inspection.Availability, diagnostics...)
		if err != nil {
			inspection.Availability = append(inspection.Availability, plugin.Diagnostic{Component: "release", Code: "preparation_failed"})
			slog.WarnContext(ctx, "committed plugin preparation failed", "installation", id, "error", err)
		}
		if err := c.connections.ReconcileInstallation(realizationCtx, reconcile); err != nil {
			slog.WarnContext(ctx, "committed plugin reconciliation failed", "installation", id, "error", err)
			inspection.Availability = append(inspection.Availability, plugin.Diagnostic{Component: "mcp", Code: "reconciliation_failed"})
		}
	}
	backends, err := c.packages.Servers(realizationCtx, committed)
	if err != nil {
		inspection.Availability = append(inspection.Availability, plugin.Diagnostic{Component: "release", Code: "unavailable_release"})
		slog.WarnContext(ctx, "committed plugin projection failed", "installation", id, "error", err)
	} else {
		inspection.Availability = append(inspection.Availability, backends.Availability...)
	}
	c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins}, invalidation.Notice{Resource: invalidation.Skills})
	return inspection, nil
}
func (c *Coordinator) Uninstall(ctx context.Context, id string) (Removal, error) {
	var names []mcpserver.ServerName
	c.mu.Lock()
	err := c.dependencies.ChangeInstallation(ctx, id, AllowInUse, func() error {
		installation, err := c.store.Get(ctx, id)
		if err != nil {
			return err
		}
		names, err = declaredNames(installation.Snapshot())
		if err != nil {
			return err
		}
		return c.store.Remove(ctx, id)
	})
	c.mu.Unlock()
	if err != nil {
		return Removal{}, err
	}
	// Removal has withdrawn admission durably. Connections retains and joins
	// teardown independently of this request, including during shutdown.
	result := Removal{}
	realizationCtx, finishRealization := c.realizationContext(ctx)
	defer finishRealization()
	if err := c.connections.ReconcileInstallation(realizationCtx, names); err != nil {
		result.Availability = append(result.Availability, plugin.Diagnostic{Component: "mcp", Code: "reconciliation_failed"})
		slog.WarnContext(ctx, "removed plugin reconciliation failed", "installation", id, "error", err)
	}
	c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins}, invalidation.Notice{Resource: invalidation.Skills})
	return result, nil
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

func declaredNames(record plugin.Record) ([]mcpserver.ServerName, error) {
	result := make([]mcpserver.ServerName, 0, len(record.Selected.Servers))
	for _, server := range record.Selected.Servers {
		name, err := mcpserver.InstallationServer(record.ID, server.Name)
		if err != nil {
			return nil, err
		}
		result = append(result, name)
	}
	return result, nil
}
