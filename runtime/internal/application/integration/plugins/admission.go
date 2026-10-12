package plugins

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/cancellation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

// Admission is the single serialization point between installation
// changes, command binding/dispatch and execution admission. Assembly runs outside it: model resolution,
// Skill listing and package reads never delay a change, and a waiting change
// never delays a new Run. Only publication of an assembled session and the
// change itself are serialized.
//
// The serialization point is a one-slot semaphore rather than a mutex: a change
// holds it across durable storage, so whoever waits behind it must still honor
// its own cancellation. Registering an assembly is not serialized with
// changes; it only touches the in-flight set, under assembliesMu, which is never
// held across I/O.
type Admission struct {
	slot         chan struct{}
	assembliesMu sync.Mutex
	assemblies   map[*Assembly]struct{}
}

// Assembly collects the installations whose execution changes
// committed while one session assembled. Those changes could not observe the
// session, and its manifest may predate the withdrawal of their superseded
// tools, so publication refuses it if it depends on any of them.
type Assembly struct {
	changed []resourceid.InstallationID
}

func NewAdmission() *Admission {
	return &Admission{slot: make(chan struct{}, 1), assemblies: make(map[*Assembly]struct{})}
}

func (a *Admission) enter(ctx context.Context) (func(), error) {
	if err := cancellation.Err(ctx); err != nil {
		return nil, err
	}
	select {
	case a.slot <- struct{}{}:
		if err := cancellation.Err(ctx); err != nil {
			<-a.slot
			return nil, err
		}
		return func() { <-a.slot }, nil
	case <-ctx.Done():
		return nil, cancellation.Err(ctx)
	}
}

func (a *Admission) Begin() (*Assembly, func()) {
	assembly := &Assembly{}
	a.assembliesMu.Lock()
	a.assemblies[assembly] = struct{}{}
	a.assembliesMu.Unlock()
	return assembly, func() {
		a.assembliesMu.Lock()
		delete(a.assemblies, assembly)
		a.assembliesMu.Unlock()
	}
}

// changed records a committed execution change against every in-flight
// assembly. The caller holds the serialization point.
func (a *Admission) changed(id resourceid.InstallationID) {
	a.assembliesMu.Lock()
	defer a.assembliesMu.Unlock()
	for assembly := range a.assemblies {
		assembly.changed = append(assembly.changed, id)
	}
}

func (a *Admission) Publish(ctx context.Context, assembly *Assembly, dependencies []plugin.Dependency, publish func() error) error {
	leave, err := a.enter(ctx)
	if err != nil {
		return err
	}
	defer leave()
	a.assembliesMu.Lock()
	if _, active := a.assemblies[assembly]; !active {
		a.assembliesMu.Unlock()
		return fmt.Errorf("%w: assembly is not active", plugin.ErrInvalid)
	}
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

func (a *Admission) Under(ctx context.Context, apply func() error) error {
	leave, err := a.enter(ctx)
	if err != nil {
		return err
	}
	defer leave()
	return apply()
}

func (a *Admission) Change(ctx context.Context, id resourceid.InstallationID, apply func() (bool, error)) error {
	return a.Under(ctx, func() error {
		changed, err := apply()
		if changed {
			a.changed(id)
		}
		return err
	})
}
