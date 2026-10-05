package execution

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

type InstallationCheckpoints interface {
	PendingCheckpointDependencies(context.Context) ([]plugin.Dependency, error)
}

// installationAdmission is the single serialization point between installation
// changes and execution admission. Assembly runs outside it: model resolution,
// Skill listing and package reads never delay a change, and a waiting change
// never delays a new Run. Only publication of an assembled session and the
// change itself are serialized.
//
// The serialization point is a one-slot semaphore rather than a mutex: a change
// holds it across durable storage, so whoever waits behind it must still honor
// its own cancellation. Registering an assembly is not serialized with
// changes; it only touches the in-flight set, under assembliesMu, which is never
// held across I/O.
type installationAdmission struct {
	slot         chan struct{}
	assembliesMu sync.Mutex
	assemblies   map[*installationAssembly]struct{}
}

// installationAssembly collects the installations whose quiescent changes
// committed while one session assembled. Those changes could not observe the
// session, and its manifest may predate the withdrawal of their superseded
// tools, so publication refuses it if it depends on any of them.
type installationAssembly struct {
	changed []resourceid.InstallationID
}

func newInstallationAdmission() *installationAdmission {
	return &installationAdmission{slot: make(chan struct{}, 1), assemblies: make(map[*installationAssembly]struct{})}
}

// enter takes the serialization point or returns the cause of ctx.
func (a *installationAdmission) enter(ctx context.Context) (func(), error) {
	select {
	case a.slot <- struct{}{}:
		return func() { <-a.slot }, nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

func (a *installationAdmission) begin() (*installationAssembly, func()) {
	assembly := &installationAssembly{}
	a.assembliesMu.Lock()
	a.assemblies[assembly] = struct{}{}
	a.assembliesMu.Unlock()
	return assembly, func() {
		a.assembliesMu.Lock()
		delete(a.assemblies, assembly)
		a.assembliesMu.Unlock()
	}
}

// changed records a committed quiescent change against every in-flight
// assembly. The caller holds the serialization point.
func (a *installationAdmission) changed(id resourceid.InstallationID) {
	a.assembliesMu.Lock()
	defer a.assembliesMu.Unlock()
	for assembly := range a.assemblies {
		assembly.changed = append(assembly.changed, id)
	}
}

func (a *installationAdmission) publish(ctx context.Context, assembly *installationAssembly, dependencies []plugin.Dependency, publish func() error) error {
	leave, err := a.enter(ctx)
	if err != nil {
		return err
	}
	defer leave()
	a.assembliesMu.Lock()
	delete(a.assemblies, assembly)
	changed := slices.Clone(assembly.changed)
	a.assembliesMu.Unlock()
	for _, dependency := range dependencies {
		if slices.Contains(changed, dependency.InstallationID) {
			return fmt.Errorf("execution: installation %s changed during assembly: %w", dependency.InstallationID, runs.ErrInstallationChanged)
		}
	}
	return publish()
}

// ChangeInstallation applies one installation change at the admission
// serialization point. Quiescence is derived from the dependency projections of
// owned sessions and pending checkpoints, never from a plugin refcount; change
// receives that same held set, so whatever it reclaims is decided against the
// state that admission serializes. change commits durably and withdraws the
// superseded live tools before it returns, so a session assembled after this
// call observes only the committed state. A failing change may already have
// committed, so in-flight assemblies are marked whenever it ran; a spurious mark
// costs only a retryable refusal.
func (i *InteractionExecutor) ChangeInstallation(ctx context.Context, id resourceid.InstallationID, admission plugins.ChangeAdmission, change func(held []plugin.Dependency) error) error {
	if admission != plugins.RequireQuiescent && admission != plugins.AllowInUse {
		return plugin.ErrInvalid
	}
	leave, err := i.installations.enter(ctx)
	if err != nil {
		return err
	}
	defer leave()
	held, err := i.heldDependencies(ctx)
	if err != nil {
		return err
	}
	if admission == plugins.AllowInUse {
		return change(held)
	}
	if slices.ContainsFunc(held, func(dependency plugin.Dependency) bool { return dependency.InstallationID == id }) {
		return plugin.ErrInUse
	}
	err = change(held)
	i.installations.changed(id)
	return err
}

// UnderAdmission runs inspect at the admission serialization point with the
// releases execution holds, without changing any installation.
func (i *InteractionExecutor) UnderAdmission(ctx context.Context, inspect func(held []plugin.Dependency) error) error {
	leave, err := i.installations.enter(ctx)
	if err != nil {
		return err
	}
	defer leave()
	held, err := i.heldDependencies(ctx)
	if err != nil {
		return err
	}
	return inspect(held)
}

// heldDependencies is the canonical union of every owned session's dependency
// projection and every pending checkpoint's. The serialization point is held.
func (i *InteractionExecutor) heldDependencies(ctx context.Context) ([]plugin.Dependency, error) {
	if i.config.InstallationCheckpoints == nil {
		return nil, errors.New("execution: installation checkpoint reader is required")
	}
	held, err := i.config.InstallationCheckpoints.PendingCheckpointDependencies(ctx)
	if err != nil {
		return nil, err
	}
	for _, session := range i.sessions.snapshot() {
		held = append(held, session.installationDependencies()...)
	}
	return plugin.CompactDependencies(held), nil
}

// installationDependencies is the session's single dependency projection: the
// canonical union of its frozen manifests' installation releases.
func (i *interactionSession) installationDependencies() []plugin.Dependency {
	i.state.mu.Lock()
	deployments := i.state.deployments
	i.state.mu.Unlock()
	if deployments == nil {
		return nil
	}
	var dependencies []plugin.Dependency
	for _, manifest := range deployments.manifests {
		dependencies = append(dependencies, manifest.Installations...)
	}
	return plugin.CompactDependencies(dependencies)
}

func installationDependencies(dependencies []plugin.Dependency) []installationDependencyWire {
	result := make([]installationDependencyWire, 0, len(dependencies))
	for _, dependency := range dependencies {
		result = append(result, installationDependencyWire{InstallationID: dependency.InstallationID.String(), Digest: dependency.Digest.String()})
	}
	return result
}
