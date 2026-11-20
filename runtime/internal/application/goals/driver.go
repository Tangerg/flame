// Package goals owns autonomous Goal execution: given a
// session's objective, it launches runs back-to-back until the model signals the
// goal complete or blocked (through terminal outcome reporting), or the user
// stops it. It is a Run-admission client with an
// autonomous application component that drives the runs Coordinator — but is
// event-driven per goal rather than cron-timed, and consumes each run's terminal
// to decide whether to continue.
//
// Goal driving lives here, NOT in the run pump: the pump holds the session's single
// admission slot across its teardown, so re-entering the coordinator from inside
// it would deadlock. The Driver launches the next run only after the previous
// run's stream has fully drained.
package goals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/taskgroup"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

var (
	// ErrGoalActive reports a start attempt while the session already has an
	// actively-driving goal.
	ErrGoalActive = errors.New("goals: a goal is already active for this session")
	// ErrNoGoal reports a resume or stop with no goal for the session.
	ErrNoGoal = errors.New("goals: no goal for this session")
	// ErrNoSession reports a start for a session that does not exist — a goal is
	// session-owned, so it must never outlive (or precede) its session.
	ErrNoSession = errors.New("goals: session does not exist")
	// ErrGoalConflict reports that a concurrent lifecycle transition won the goal's
	// compare-and-swap; the caller read a version that was already superseded.
	ErrGoalConflict = errors.New("goals: goal changed concurrently")
	// ErrClosed reports a lifecycle command after the driver has stopped accepting
	// work. A caller must never be told an active Goal was accepted when no drive
	// can be attached to drive it.
	ErrClosed = errors.New("goals: driver closed")
)

// AutonomousRuns is the Goal driver's narrow view of the Run entry point.
type AutonomousRuns interface {
	WaitSessionStartable(ctx context.Context, sessionID identity.SessionID) error
	AdmitSelection(ctx context.Context, selection modelchoice.Selection) error
	Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error)
}

// RunCancellations stops a Goal's autonomous Run when the drive is superseded.
type RunCancellations interface {
	// Cancel returns after the Run has reached its complete terminal boundary.
	Cancel(ctx context.Context, cmd runs.CancelCommand) (runs.CancelResult, error)
}

// SessionExistence reports whether a Session exists. A Goal reads nothing else
// from it: without its own override a Goal Run uses the Session's selection
// when it starts.
type SessionExistence interface {
	Exists(ctx context.Context, sessionID identity.SessionID) (bool, error)
}

// Driver owns the per-session autonomous drives. Each active Goal has at most one
// drive goroutine, spawned into a task group so shutdown cancels and joins them.
//
// Every durable goal write is a compare-and-swap on [goal.Version]. An opaque
// incarnation distinguishes fresh objectives across clears, and the revision
// protects mutations inside one incarnation. Per-session command locks serialize explicit
// lifecycle commands with session write-sets without coupling unrelated
// sessions; drive goroutines and reported outcomes use the store CAS.
type Driver struct {
	workflow       Workflow
	settle         func(context.Context, identity.SessionID) error
	goals          Store
	runs           AutonomousRuns
	cancellations  RunCancellations
	sessions       SessionExistence
	tasks          *taskgroup.Group
	now            func() time.Time
	newIncarnation func() string

	mutations *SessionMutations
}

// NewDriver builds a Driver sharing one session lifecycle
// coordinator with the sessions use case.
func NewDriver(
	store Store,
	autonomousRuns AutonomousRuns,
	cancellations RunCancellations,
	sessions SessionExistence,
	mutations *SessionMutations,
	workflow Workflow, settle func(context.Context, identity.SessionID) error,
) (*Driver, error) {
	if lo.IsNil(workflow) || settle == nil {
		return nil, errors.New("goals: workflow and receipt settlement are required")
	}
	if lo.IsNil(store) {
		return nil, errors.New("goals: store is required")
	}
	if lo.IsNil(autonomousRuns) {
		return nil, errors.New("goals: autonomous runs are required")
	}
	if lo.IsNil(cancellations) {
		return nil, errors.New("goals: run cancellations are required")
	}
	if lo.IsNil(sessions) {
		return nil, errors.New("goals: session policy reader is required")
	}
	if mutations == nil {
		return nil, errors.New("goals: shared session mutations are required")
	}
	return &Driver{workflow: workflow, settle: settle,
		goals:          store,
		runs:           autonomousRuns,
		cancellations:  cancellations,
		sessions:       sessions,
		tasks:          &taskgroup.Group{},
		now:            time.Now,
		newIncarnation: uuid.NewString,
		mutations:      mutations,
	}, nil
}

// Start opens a new goal for the session and begins driving it. It replaces a
// paused or blocked goal (a fresh objective abandons the old one) but refuses to
// clobber a goal that is already actively driving, and refuses a session that
// does not exist. The new objective gets a fresh incarnation so a Run from any
// previously-cleared Goal can no longer write it.
func (d *Driver) Start(
	ctx context.Context,
	sessionID identity.SessionID, objective string,
	selection modelchoice.Selection,
	capabilities run.Capabilities,
) (goal.Goal, error) {
	release, err := d.mutations.acquire(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	defer release()
	if d.tasks.Closed() {
		return goal.Goal{}, ErrClosed
	}
	exists, err := d.sessions.Exists(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if !exists {
		return goal.Goal{}, ErrNoSession
	}
	if selection.Configured() {
		if admissionErr := d.runs.AdmitSelection(ctx, selection); admissionErr != nil {
			return goal.Goal{}, admissionErr
		}
	}
	existing, ok, err := loadGoal(ctx, d.goals, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if ok && existing.Status() == goal.StatusActive {
		return goal.Goal{}, ErrGoalActive
	}
	unwritten, err := goal.Unwritten(sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	expected := unwritten.Version()
	if ok {
		expected = existing.Version()
	}
	g, err := d.beginGoal(ctx, sessionID, objective, selection, capabilities)
	if err != nil {
		return goal.Goal{}, err
	}
	return d.commitCommand(ctx, expected, g)
}

func (d *Driver) beginGoal(ctx context.Context, sessionID identity.SessionID, objective string, selection modelchoice.Selection, capabilities run.Capabilities) (goal.Goal, error) {
	now := d.now()
	state := goal.Snapshot{SessionID: sessionID, ModelSelection: selection, Capabilities: capabilities.Normalized(), CreatedAt: now}
	g, changed, err := decideGoal(ctx, d.workflow, state, goalworkflow.Request{Signal: goalworkflow.Start, Objective: objective}, d.newIncarnation(), now)
	if err == nil && !changed {
		return goal.Goal{}, errors.New("goals: workflow omitted its initial objective")
	}
	return g, err
}

// Resume preserves the objective incarnation so a parked Run can still settle
// its original Goal. An already-active Goal can reattach its missing drive.
func (d *Driver) Resume(ctx context.Context, sessionID identity.SessionID, caller run.Capabilities) (goal.Goal, error) {
	release, err := d.mutations.acquire(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	defer release()
	if d.tasks.Closed() {
		return goal.Goal{}, ErrClosed
	}
	g, ok, err := loadGoal(ctx, d.goals, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if !ok {
		return goal.Goal{}, ErrNoGoal
	}
	if missing := g.Capabilities().MissingFrom(caller); !missing.IsEmpty() {
		return goal.Goal{}, fmt.Errorf("goals: session %q: %w", sessionID, &run.InsufficientCapabilitiesError{Missing: missing})
	}
	if g.Status() == goal.StatusActive {
		if err := d.workflow.RequireGoalAuthor(ctx, g.Snapshot().Author); err != nil {
			return goal.Goal{}, err
		}
		if ensureDriveLockedErr := d.ensureDriveLocked(ctx, sessionID, g.IncarnationID()); ensureDriveLockedErr != nil {
			return goal.Goal{}, ensureDriveLockedErr
		}
		return g, nil
	}
	replacement, err := transition(ctx, d.workflow, g, goalworkflow.Request{Signal: goalworkflow.Resume}, g.IncarnationID(), d.now())
	if err != nil {
		return goal.Goal{}, err
	}
	return d.commitCommand(ctx, g.Version(), replacement)
}

func (d *Driver) Stop(ctx context.Context, sessionID identity.SessionID) (goal.Goal, error) {
	release, err := d.mutations.acquire(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	defer release()
	if d.tasks.Closed() {
		return goal.Goal{}, ErrClosed
	}
	current, present, err := loadGoal(ctx, d.goals, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if !present {
		return goal.Goal{}, ErrNoGoal
	}
	if current.Status() == goal.StatusActive {
		paused, err := transition(ctx, d.workflow, current, goalworkflow.Request{Signal: goalworkflow.Stop}, current.IncarnationID(), d.now())
		if err != nil {
			return goal.Goal{}, err
		}
		if paused.Status() != goal.StatusPaused {
			return goal.Goal{}, fmt.Errorf("%w: workflow stop must pause the Goal", goal.ErrInvalid)
		}
		if err := d.saveCommand(ctx, current.Version(), paused); err != nil {
			return goal.Goal{}, err
		}
	}
	// The stop receipt includes usage from the original Run's terminal write.
	// A disconnected caller cannot interrupt that observation after the pause.
	settlement, cancel := context.WithTimeout(context.WithoutCancel(ctx), goalRunCleanupTimeout)
	defer cancel()
	if err := d.mutations.joinDrive(settlement, sessionID, d.mutations.quiesce(sessionID)); err != nil {
		return goal.Goal{}, fmt.Errorf("goals: observe stopped Goal drive settlement: %w", err)
	}
	settled, present, err := loadGoal(settlement, d.goals, sessionID)
	if err != nil {
		return goal.Goal{}, fmt.Errorf("goals: read stopped Goal: %w", err)
	}
	if !present {
		return goal.Goal{}, ErrNoGoal
	}
	return settled, nil
}

// UpdateObjective opens a fresh incarnation so work admitted for the previous
// objective cannot decide or charge the replacement.
func (d *Driver) UpdateObjective(
	ctx context.Context,
	sessionID identity.SessionID, objective string,
	caller run.Capabilities,
) (goal.Goal, error) {
	release, err := d.mutations.acquire(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	defer release()
	if d.tasks.Closed() {
		return goal.Goal{}, ErrClosed
	}
	current, present, err := loadGoal(ctx, d.goals, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if !present {
		return goal.Goal{}, ErrNoGoal
	}
	if current.Status() == goal.StatusComplete {
		return goal.Goal{}, goal.ErrNotEditable
	}
	if current.Status() == goal.StatusActive {
		if missing := current.Capabilities().MissingFrom(caller); !missing.IsEmpty() {
			return goal.Goal{}, fmt.Errorf("goals: session %q: %w", sessionID, &run.InsufficientCapabilitiesError{Missing: missing})
		}
	}
	replacement, err := transition(ctx, d.workflow, current, goalworkflow.Request{Signal: goalworkflow.Revise, Objective: objective}, d.newIncarnation(), d.now())
	if err != nil {
		return goal.Goal{}, err
	}
	return d.commitCommand(ctx, current.Version(), replacement)
}

// A superseded decision has no authority to cancel a drive. Only a committed
// Goal authorizes replacing or retiring the process-local execution owner.
func (d *Driver) commitCommand(ctx context.Context, expected goal.Version, replacement goal.Goal) (goal.Goal, error) {
	if err := d.saveCommand(ctx, expected, replacement); err != nil {
		return goal.Goal{}, err
	}
	sessionID := replacement.SessionID()
	if replacement.Status() == goal.StatusActive {
		if err := d.launchLocked(ctx, sessionID, replacement.IncarnationID()); err != nil {
			panic("goals: command crossed the shutdown admission boundary")
		}
	} else {
		d.retireDrive(ctx, sessionID)
	}
	return replacement, nil
}

func (d *Driver) saveCommand(ctx context.Context, expected goal.Version, replacement goal.Goal) error {
	change, err := goal.NewReplacement(expected, replacement)
	if err != nil {
		return err
	}
	applied, err := d.goals.Save(ctx, change)
	if err != nil {
		return err
	}
	if !applied {
		return ErrGoalConflict
	}
	return nil
}

// Clear commits the Goal's removal before retiring its drive. An already-absent
// Goal lets a clear converge with automatic completion.
func (d *Driver) Clear(ctx context.Context, sessionID identity.SessionID) error {
	release, err := d.mutations.acquire(ctx, sessionID)
	if err != nil {
		return err
	}
	defer release()
	if d.tasks.Closed() {
		return ErrClosed
	}
	current, present, err := loadGoal(ctx, d.goals, sessionID)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	applied, err := d.goals.ClearIf(ctx, sessionID, current.Version())
	if err != nil {
		return err
	}
	if !applied {
		return ErrGoalConflict
	}
	d.retireDrive(ctx, sessionID)
	return nil
}

// Current returns the session's Goal, or (zero, false, nil) when it has none.
func (d *Driver) Current(ctx context.Context, sessionID identity.SessionID) (goal.Goal, bool, error) {
	return loadGoal(ctx, d.goals, sessionID)
}

// Reconcile clears orphaned and completed Goals and pauses active Goals whose
// drives ended. The shared Session command boundary excludes a concurrent
// launch; the drive registry alone proves liveness inside Runtime's exclusive
// data-directory lifetime. Every durable transition still uses the listed
// version and fails closed on a CAS miss.
func (d *Driver) Reconcile(ctx context.Context) error {
	all, err := d.goals.List(ctx)
	if err != nil {
		return err
	}
	for _, g := range all {
		if err := d.reconcileGoal(ctx, g); err != nil {
			return err
		}
	}
	return nil
}

func (d *Driver) reconcileGoal(ctx context.Context, g goal.Goal) error {
	release, err := d.mutations.acquireSessions(ctx, g.SessionID())
	if err != nil {
		return err
	}
	defer release()
	if drive := d.mutations.activeDrive(g.SessionID()); drive != nil && !drive.completed() {
		return nil
	}
	if err := d.settle(ctx, g.SessionID()); err != nil {
		if errors.Is(err, plugin.ErrUnavailable) {
			return nil
		}
		return err
	}
	fresh, found, err := loadGoal(ctx, d.goals, g.SessionID())
	if err != nil || !found {
		return err
	}
	g = fresh
	exists, err := d.sessions.Exists(ctx, g.SessionID())
	if err != nil {
		return err
	}
	var applied bool
	switch {
	case !exists || g.Status() == goal.StatusComplete:
		applied, err = d.goals.ClearIf(ctx, g.SessionID(), g.Version())
	case g.Status() == goal.StatusActive:
		applied, err = d.pauseAfterRestart(ctx, g)
	default:
		return nil
	}
	if err != nil || applied {
		return err
	}
	return ErrGoalConflict
}

// pauseAfterRestart pauses a Goal the previous process left active. A
// workflow that is not available yet leaves it for the next reconciliation.
func (d *Driver) pauseAfterRestart(ctx context.Context, g goal.Goal) (bool, error) {
	replacement, err := transition(ctx, d.workflow, g, goalworkflow.Request{Signal: goalworkflow.Pause, Reason: goalworkflow.GoalReasonRuntimeRestarted}, g.IncarnationID(), d.now())
	if errors.Is(err, plugin.ErrUnavailable) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	change, err := goal.NewReplacement(g.Version(), replacement)
	if err != nil {
		return false, err
	}
	return d.goals.Save(ctx, change)
}

// BeginShutdown cancels every running Goal drive.
func (d *Driver) BeginShutdown() {
	release := d.mutations.acquireAll()
	defer release()
	d.tasks.Cancel()
}

// AwaitShutdown joins every running Goal drive after [BeginShutdown].
func (d *Driver) AwaitShutdown(ctx context.Context) error {
	return d.tasks.Wait(ctx)
}
