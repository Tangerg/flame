package schedules

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
)

// RunNowStore is the on-demand firing persistence slice.
type RunNowStore interface {
	Get(ctx context.Context, id string) (schedule.Schedule, error)
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
	newSessionID  func() string
	newRunID      func() string
	now           func() time.Time
	invalidations invalidation.Publish
}

// FiringDependencies is the complete collaborator set for manual and cron
// schedule execution.
type FiringDependencies struct {
	Store         FiringStore
	RunStarter    ScheduledRunStarter
	NewSessionID  func() string
	NewRunID      func() string
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
		{name: "occurrence Session identity factory", value: deps.NewSessionID},
		{name: "occurrence Run identity factory", value: deps.NewRunID},
	} {
		if dependencyMissing(required.value) {
			return nil, fmt.Errorf("schedules: firing %s is required", required.name)
		}
	}
	return &Firing{
		runNowStore: deps.Store, workerStore: deps.Store, runStarter: deps.RunStarter,
		newSessionID: deps.NewSessionID, newRunID: deps.NewRunID,
		now: time.Now, invalidations: deps.Invalidations,
	}, nil
}

// Available reports whether schedule-firing use cases are wired.
func (f *Firing) Available() bool {
	return f != nil && f.runNowStore != nil
}

// RunNow starts one off-cycle schedule firing without advancing the cron
// cursor. The request carries its aggregate-owned Run record into the same
// transaction as Run opening, so success and LastRunAt cannot diverge.
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
	request, err := schedule.ManualRunRequest(scheduled, f.newSessionID(), f.newRunID(), f.now())
	if err != nil {
		return StartedRun{}, err
	}
	startedRun, err := Fire(ctx, f.runStarter, request)
	if err != nil {
		return StartedRun{}, err
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
		Store: f.workerStore, RunStarter: f.runStarter,
		NewSessionID: f.newSessionID, NewRunID: f.newRunID,
		Invalidations: f.invalidations,
	}).Run(ctx)
}
