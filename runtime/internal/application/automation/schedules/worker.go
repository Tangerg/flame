package schedules

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
)

var workerTracer = otel.Tracer("scope/flame/schedule")

const workerTick = time.Minute

// workerBatchSize bounds the durable work admitted by one ticker pass. Pending
// retries rotate through durable ordering. Newly due schedules are claimed and
// dispatched together so shutdown cannot materialize an unbounded backlog.
const workerBatchSize = 32

// ScheduledRunStarter starts one scheduled instruction set as a headless run. It is the
// application-owned seam between a fired schedule and a run start.
type ScheduledRunStarter interface {
	StartScheduledRun(ctx context.Context, request schedule.RunRequest) error
}

// StartedRun identifies the Run accepted for one schedule occurrence.
type StartedRun struct {
	SessionID string
	RunID     string
}

// WorkerStore is the schedule persistence slice the worker owns. Management
// CRUD stays on the management use case; the worker claims due occurrences and
// re-drives the durable pending work items it previously materialized.
type WorkerStore interface {
	Due(ctx context.Context, now time.Time, limit int) ([]schedule.Schedule, error)
	Claim(ctx context.Context, claim schedule.Claim) (claimed bool, err error)
	Pending(ctx context.Context, afterDueAt time.Time, afterID string, limit int) ([]schedule.Occurrence, error)
}

// workerDependencies is the complete collaborator set for a due scanner.
type workerDependencies struct {
	Store         WorkerStore
	RunStarter    ScheduledRunStarter
	NewSessionID  func() string
	NewRunID      func() string
	Invalidations invalidation.Publish
}

// worker scans due schedules, atomically materializes occurrence work items,
// and dispatches pending work. Its retry cursor belongs to one Run loop. It is the ticker component of the automation
// use case — the schedule spec and next-fire rule are the domain's
// ([schedule.Schedule] / [schedule.NextRun]); the periodic scan and side-effecting
// firing are the application's.
type worker struct {
	pendingAfterDueAt time.Time
	pendingAfterID    string
	schedules         WorkerStore
	runStarter        ScheduledRunStarter
	newSessionID      func() string
	newRunID          func() string
	now               func() time.Time
	invalidations     invalidation.Publish
}

func newWorker(deps workerDependencies) *worker {
	return &worker{
		schedules: deps.Store, runStarter: deps.RunStarter,
		newSessionID: deps.NewSessionID, newRunID: deps.NewRunID,
		now: time.Now, invalidations: deps.Invalidations,
	}
}

// Run starts the scheduled-run loop until ctx is canceled.
func (w *worker) Run(ctx context.Context) {
	w.fireDue(ctx, w.now())
	t := time.NewTicker(workerTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.fireDue(ctx, w.now())
		}
	}
}

// Fire starts one schedule RunRequest through runner under the firing span. A
// request with no occurrence identity is manual and does not consume a cursor.
func Fire(ctx context.Context, runStarter ScheduledRunStarter, request schedule.RunRequest) (StartedRun, error) {
	if runStarter == nil {
		return StartedRun{}, errors.New("schedules: scheduled run starter is nil")
	}
	if err := request.Validate(); err != nil {
		return StartedRun{}, fmt.Errorf("schedules: invalid run request: %w", err)
	}
	ctx, span := workerTracer.Start(ctx, "schedule.fire",
		trace.WithAttributes(attribute.String("schedule.id", request.ScheduleID())))
	defer span.End()
	err := runStarter.StartScheduledRun(ctx, request)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "start run")
		return StartedRun{}, err
	}
	return StartedRun{SessionID: request.SessionID(), RunID: request.RunID()}, nil
}

func (w *worker) fireDue(ctx context.Context, now time.Time) {
	if ctx.Err() != nil {
		return
	}
	occurrences, err := w.schedules.Pending(ctx, w.pendingAfterDueAt, w.pendingAfterID, workerBatchSize/2)
	if err != nil {
		if !errors.Is(err, ctx.Err()) {
			slog.ErrorContext(ctx, "schedule: pending query failed", "error", err)
		}
		return
	}
	// Reserve at least half the pass for newly due work, and seek past failed
	// pending work on the next pass so later durable occurrences also advance.
	if len(occurrences) < workerBatchSize/2 {
		w.pendingAfterDueAt, w.pendingAfterID = time.Time{}, ""
	} else {
		last := occurrences[len(occurrences)-1]
		w.pendingAfterDueAt, w.pendingAfterID = last.DueAt(), last.ID()
	}

	batch := occurrenceBatch{ctx: ctx, runStarter: w.runStarter, invalidations: w.invalidations}
	if !batch.dispatchAll(occurrences) || batch.full() {
		return
	}
	due, err := w.schedules.Due(ctx, now, batch.remaining())
	if err != nil {
		if !errors.Is(err, ctx.Err()) {
			slog.ErrorContext(ctx, "schedule: due query failed", "error", err)
		}
		return
	}

	for _, scheduled := range due {
		if batch.full() {
			return
		}
		if ctx.Err() != nil {
			return
		}
		occurrence, claimed := w.claimDueOccurrence(ctx, scheduled, now)
		if claimed && !batch.dispatch(occurrence) {
			return
		}
	}
}

type occurrenceBatch struct {
	ctx           context.Context
	runStarter    ScheduledRunStarter
	dispatched    int
	invalidations invalidation.Publish
}

func (o *occurrenceBatch) remaining() int { return workerBatchSize - o.dispatched }

func (o *occurrenceBatch) full() bool { return o.remaining() == 0 }

func (o *occurrenceBatch) dispatchAll(occurrences []schedule.Occurrence) bool {
	for _, occurrence := range occurrences {
		if o.full() || !o.dispatch(occurrence) {
			return false
		}
	}
	return true
}

func (o *occurrenceBatch) dispatch(occurrence schedule.Occurrence) bool {
	if o.ctx.Err() != nil {
		return false
	}
	o.dispatched++
	_, err := Fire(o.ctx, o.runStarter, occurrence.RunRequest())
	if err == nil {
		o.invalidations.Notify(invalidation.ForSchedules(occurrence.ScheduleID()))
	}
	if err != nil && o.ctx.Err() != nil && errors.Is(err, o.ctx.Err()) {
		return false
	}
	if err != nil {
		slog.ErrorContext(
			o.ctx,
			"schedule: run start failed",
			"schedule.id", occurrence.ScheduleID(), "error", err,
		)
	}
	return true
}

func (w *worker) claimDueOccurrence(
	ctx context.Context,
	scheduled schedule.Schedule,
	now time.Time,
) (schedule.Occurrence, bool) {
	claim, err := schedule.NewClaim(
		scheduled,
		w.newSessionID(),
		w.newRunID(),
		now,
	)
	if err != nil {
		slog.ErrorContext(ctx, "schedule: prepare due occurrence failed", "schedule.id", scheduled.ID(), "error", err)
		return schedule.Occurrence{}, false
	}
	claimed, err := w.schedules.Claim(ctx, claim)
	if err != nil {
		if !errors.Is(err, ctx.Err()) {
			slog.ErrorContext(ctx, "schedule: claim due occurrence failed", "schedule.id", scheduled.ID(), "error", err)
		}
		return schedule.Occurrence{}, false
	}
	if claimed {
		// Claim advances NextRunAt before Run admission. Publish that committed
		// cursor even when the following start fails; a later pending retry that is
		// accepted publishes again for LastRunAt.
		w.invalidations.Notify(invalidation.ForSchedules(scheduled.ID()))
	}
	return claim.Occurrence(), claimed
}
