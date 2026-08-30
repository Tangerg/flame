package schedules

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/schedule"
)

// RunNowStore is the on-demand firing persistence slice.
type RunNowStore interface {
	Get(ctx context.Context, id string) (schedule.Schedule, error)
	RecordRun(ctx context.Context, record schedule.RunRecord) error
}

// FiringStore joins the independently consumed run-now and worker slices for
// wiring one persistence implementation into this application component.
type FiringStore interface {
	RunNowStore
	WorkerStore
}

// Firing owns schedule execution after a management operation or worker tick.
// It is constructed with a complete ScheduledRunStarter, so callers cannot observe an
// incompletely wired scheduler.
type Firing struct {
	runNowStore   RunNowStore
	workerStore   WorkerStore
	runStarter    ScheduledRunStarter
	identities    OccurrenceIdentities
	now           func() time.Time
	invalidations invalidation.Publish
}

// FiringDependencies is the complete collaborator set for manual and cron
// schedule execution.
type FiringDependencies struct {
	Store         FiringStore
	RunStarter    ScheduledRunStarter
	Identities    OccurrenceIdentities
	Invalidations invalidation.Publish
}

// DisabledFiring returns an explicitly unavailable execution capability.
func DisabledFiring() *Firing { return &Firing{} }

// NewFiring builds a complete schedule execution use case and rejects partial
// construction.
func NewFiring(deps FiringDependencies) (*Firing, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{name: "store", value: deps.Store},
		{name: "run starter", value: deps.RunStarter},
		{name: "occurrence identities", value: deps.Identities},
	} {
		if dependencyMissing(required.value) {
			return nil, fmt.Errorf("schedules: firing %s is required", required.name)
		}
	}
	return &Firing{
		runNowStore: deps.Store, workerStore: deps.Store, runStarter: deps.RunStarter,
		identities: deps.Identities, now: time.Now, invalidations: deps.Invalidations,
	}, nil
}

// Available reports whether schedule-firing use cases are wired.
func (f *Firing) Available() bool {
	return f != nil && f.runNowStore != nil
}

// RunNow starts one off-cycle schedule firing and records it without advancing
// the cron cursor. Once accepted, recording outlives request cancellation so a
// durable LastRunAt fact cannot be lost after a client disconnect.
func (f *Firing) RunNow(ctx context.Context, id string) (StartedRun, error) {
	if !f.Available() {
		return StartedRun{}, ErrUnavailable
	}
	if err := schedule.ValidateID(id); err != nil {
		return StartedRun{}, err
	}
	scheduled, err := f.runNowStore.Get(ctx, id)
	if err != nil {
		return StartedRun{}, err
	}
	record, err := scheduled.RecordRun(f.now())
	if err != nil {
		return StartedRun{}, fmt.Errorf("schedules: form run-now record for %q: %w", id, err)
	}
	request, err := schedule.ManualRunRequest(scheduled)
	if err != nil {
		return StartedRun{}, err
	}
	startedRun, err := Fire(ctx, f.runStarter, request)
	if err != nil {
		return StartedRun{}, err
	}

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), manualRunRecordTimeout)
	defer cancel()
	if err := f.runNowStore.RecordRun(writeCtx, record); err != nil {
		return StartedRun{}, fmt.Errorf("schedules: record run-now for %q: %w", id, err)
	}
	f.invalidations.Notify(invalidation.ForSchedules(id))
	return startedRun, nil
}

// RunWorker starts the due-schedule scanner until ctx is canceled.
func (f *Firing) RunWorker(ctx context.Context) {
	if !f.Available() {
		return
	}
	newWorker(workerDependencies{
		Store: f.workerStore, RunStarter: f.runStarter, Identities: f.identities,
		Invalidations: f.invalidations,
	}).Run(ctx)
}
