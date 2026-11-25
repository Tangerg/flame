// Goal driving lives outside the run pump: the pump holds the session's admission
// slot across teardown, so re-entering the coordinator from inside it would
// deadlock.
package goals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"

	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/application/taskgroup"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

var (
	ErrGoalActive   = errors.New("goals: a goal is already active for this session")
	ErrNoGoal       = errors.New("goals: no goal for this session")
	ErrNoSession    = errors.New("goals: session does not exist")
	ErrGoalConflict = errors.New("goals: goal changed concurrently")
	ErrClosed       = errors.New("goals: driver closed")
)

type AutonomousRuns interface {
	WaitSessionStartable(ctx context.Context, sessionID identity.SessionID) error
	AdmitSelection(ctx context.Context, selection flamemodel.Selection) error
	Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error)
}

type RunCancellations interface {
	Cancel(ctx context.Context, cmd runs.CancelCommand) (runs.CancelResult, error)
}

type SessionExistence interface {
	Exists(ctx context.Context, sessionID identity.SessionID) (bool, error)
}

type Driver struct {
	workflow       Workflow
	settle         func(context.Context, identity.SessionID) error
	goals          Store
	runs           AutonomousRuns
	cancellations  RunCancellations
	sessions       SessionExistence
	invalidations  invalidation.Publish
	tasks          *taskgroup.Group
	now            func() time.Time
	newIncarnation func() (goalref.IncarnationID, error)

	mutations *SessionMutations
}

func NewDriver(
	store Store,
	autonomousRuns AutonomousRuns,
	cancellations RunCancellations,
	sessions SessionExistence,
	mutations *SessionMutations,
	invalidations invalidation.Publish,
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
	if invalidations == nil {
		return nil, errors.New("goals: invalidations are required")
	}
	return &Driver{workflow: workflow, settle: settle,
		goals:          store,
		runs:           autonomousRuns,
		cancellations:  cancellations,
		sessions:       sessions,
		invalidations:  invalidations,
		tasks:          &taskgroup.Group{},
		now:            time.Now,
		newIncarnation: func() (goalref.IncarnationID, error) { return goalref.ParseIncarnation(uuid.NewString()) },
		mutations:      mutations,
	}, nil
}

func (d *Driver) Start(
	ctx context.Context,
	sessionID identity.SessionID, objective string,
	selection flamemodel.Selection,
	capabilities run.Capabilities,
	complete operation.Reply[goal.Goal],
) error {
	release, err := d.admitCommand(ctx, sessionID)
	if err != nil {
		return err
	}
	defer release()
	exists, err := d.sessions.Exists(ctx, sessionID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNoSession
	}
	if selection.Configured() {
		if admissionErr := d.runs.AdmitSelection(ctx, selection); admissionErr != nil {
			return admissionErr
		}
	}
	existing, ok, err := loadGoal(ctx, d.goals, sessionID)
	if err != nil {
		return err
	}
	if ok && existing.Status() == goal.StatusActive {
		return ErrGoalActive
	}
	unwritten, err := goal.Unwritten(sessionID)
	if err != nil {
		return err
	}
	expected := unwritten.Version()
	if ok {
		expected = existing.Version()
	}
	g, err := d.beginGoal(ctx, sessionID, objective, selection, capabilities)
	if err != nil {
		return err
	}
	return d.replaceAndDrive(ctx, expected, g, complete)
}

func (d *Driver) beginGoal(ctx context.Context, sessionID identity.SessionID, objective string, selection flamemodel.Selection, capabilities run.Capabilities) (goal.Goal, error) {
	now := d.now()
	state := goal.Snapshot{SessionID: sessionID, ModelSelection: selection, Capabilities: capabilities.Normalized(), CreatedAt: now}
	incarnation, err := d.newIncarnation()
	if err != nil {
		return goal.Goal{}, err
	}
	g, changed, err := decideGoal(ctx, d.workflow, state, goalworkflow.Request{Signal: goalworkflow.Start, Objective: objective}, incarnation, now)
	if err == nil && !changed {
		return goal.Goal{}, errors.New("goals: workflow omitted its initial objective")
	}
	return g, err
}

func (d *Driver) Resume(ctx context.Context, sessionID identity.SessionID, caller run.Capabilities, complete operation.Reply[goal.Goal]) error {
	release, err := d.admitCommand(ctx, sessionID)
	if err != nil {
		return err
	}
	defer release()
	g, ok, err := loadGoal(ctx, d.goals, sessionID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoGoal
	}
	if missing := g.Capabilities().MissingFrom(caller); !missing.IsEmpty() {
		return fmt.Errorf("goals: session %q: %w", sessionID, &run.InsufficientCapabilitiesError{Missing: missing})
	}
	if g.Status() == goal.StatusActive {
		return d.resumeActive(ctx, g, complete)
	}
	replacement, err := transition(ctx, d.workflow, g, goalworkflow.Request{Signal: goalworkflow.Resume}, g.IncarnationID(), d.now())
	if err != nil {
		return err
	}
	return d.replaceAndDrive(ctx, g.Version(), replacement, complete)
}

func (d *Driver) resumeActive(ctx context.Context, g goal.Goal, complete operation.Reply[goal.Goal]) error {
	if err := d.workflow.RequireGoalAuthor(ctx, g.Snapshot().Author); err != nil {
		return err
	}
	sessionID, incarnation := g.SessionID(), g.IncarnationID()
	drive := d.mutations.activeDrive(sessionID)
	restart := drive == nil || drive.completed()
	if drive != nil {
		if drive.incarnationID != incarnation {
			return ErrGoalConflict
		}
		if restart && drive.err != nil && !errors.Is(drive.err, plugin.ErrUnavailable) {
			return drive.err
		}
	}
	if err := complete(ctx, func(context.Context) (goal.Goal, error) { return g, nil }); err != nil {
		return err
	}
	if restart {
		if drive != nil {
			d.mutations.forget(sessionID, drive)
		}
		d.launchLocked(ctx, sessionID, incarnation)
	}
	return nil
}

func (d *Driver) Stop(ctx context.Context, sessionID identity.SessionID, complete operation.Reply[struct{}]) error {
	release, err := d.admitCommand(ctx, sessionID)
	if err != nil {
		return err
	}
	defer release()
	current, present, err := loadGoal(ctx, d.goals, sessionID)
	if err != nil {
		return err
	}
	if !present {
		return ErrNoGoal
	}
	var change *goal.Replacement
	if current.Status() == goal.StatusActive {
		paused, err := transition(ctx, d.workflow, current, goalworkflow.Request{Signal: goalworkflow.Stop}, current.IncarnationID(), d.now())
		if err != nil {
			return err
		}
		if paused.Status() != goal.StatusPaused {
			return fmt.Errorf("%w: workflow stop must pause the Goal", goal.ErrInvalid)
		}
		replacement, err := goal.NewReplacement(current.Version(), paused)
		if err != nil {
			return err
		}
		change = &replacement
	}
	if err := complete(ctx, func(tx context.Context) (struct{}, error) {
		if change == nil {
			return struct{}{}, nil
		}
		return struct{}{}, d.saveReplacement(tx, *change)
	}); err != nil {
		return err
	}
	if change != nil {
		d.publishChange(sessionID)
	}
	settlement, cancel := context.WithTimeout(context.WithoutCancel(ctx), goalRunCleanupTimeout)
	defer cancel()
	d.retireDrive(settlement, sessionID)
	return nil
}

// A fresh incarnation so work admitted for the previous objective cannot decide or
// charge the replacement.
func (d *Driver) UpdateObjective(
	ctx context.Context,
	sessionID identity.SessionID, objective string,
	caller run.Capabilities,
	complete operation.Reply[goal.Goal],
) error {
	release, err := d.admitCommand(ctx, sessionID)
	if err != nil {
		return err
	}
	defer release()
	current, present, err := loadGoal(ctx, d.goals, sessionID)
	if err != nil {
		return err
	}
	if !present {
		return ErrNoGoal
	}
	if current.Status() == goal.StatusComplete {
		return goal.ErrNotEditable
	}
	if current.Status() == goal.StatusActive {
		if missing := current.Capabilities().MissingFrom(caller); !missing.IsEmpty() {
			return fmt.Errorf("goals: session %q: %w", sessionID, &run.InsufficientCapabilitiesError{Missing: missing})
		}
	}
	incarnation, err := d.newIncarnation()
	if err != nil {
		return err
	}
	replacement, err := transition(ctx, d.workflow, current, goalworkflow.Request{Signal: goalworkflow.Revise, Objective: objective}, incarnation, d.now())
	if err != nil {
		return err
	}
	return d.replaceAndDrive(ctx, current.Version(), replacement, complete)
}

func (d *Driver) replaceAndDrive(ctx context.Context, expected goal.Version, replacement goal.Goal, complete operation.Reply[goal.Goal]) error {
	if err := d.commitReplacement(ctx, expected, replacement, complete); err != nil {
		return err
	}
	sessionID := replacement.SessionID()
	if replacement.Status() == goal.StatusActive {
		d.launchLocked(ctx, sessionID, replacement.IncarnationID())
	} else {
		d.retireDrive(ctx, sessionID)
	}
	return nil
}

func (d *Driver) commitReplacement(ctx context.Context, expected goal.Version, replacement goal.Goal, complete operation.Reply[goal.Goal]) error {
	change, err := goal.NewReplacement(expected, replacement)
	if err != nil {
		return err
	}
	if err := complete(ctx, func(tx context.Context) (goal.Goal, error) {
		return replacement, d.saveReplacement(tx, change)
	}); err != nil {
		return err
	}
	d.publishChange(replacement.SessionID())
	return nil
}

func (d *Driver) saveReplacement(ctx context.Context, change goal.Replacement) error {
	applied, err := d.goals.Save(ctx, change)
	if err != nil {
		return err
	}
	if !applied {
		return ErrGoalConflict
	}
	return nil
}

func (d *Driver) Clear(ctx context.Context, sessionID identity.SessionID, complete operation.Reply[struct{}]) error {
	release, err := d.admitCommand(ctx, sessionID)
	if err != nil {
		return err
	}
	defer release()
	current, present, err := loadGoal(ctx, d.goals, sessionID)
	if err != nil {
		return err
	}
	if err := complete(ctx, func(tx context.Context) (struct{}, error) {
		if !present {
			return struct{}{}, nil
		}
		applied, err := d.goals.ClearIf(tx, sessionID, current.Version())
		if err != nil {
			return struct{}{}, err
		}
		if !applied {
			return struct{}{}, ErrGoalConflict
		}
		return struct{}{}, nil
	}); err != nil {
		return err
	}
	if present {
		d.publishChange(sessionID)
		d.retireDrive(ctx, sessionID)
	}
	return nil
}

func (d *Driver) publishChange(sessionID identity.SessionID) {
	d.invalidations.Notify(invalidation.InSession(invalidation.Goals, sessionID))
}

func (d *Driver) admitCommand(ctx context.Context, sessionID identity.SessionID) (func(), error) {
	release, err := d.mutations.acquire(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if d.tasks.Closed() {
		release()
		return nil, ErrClosed
	}
	return release, nil
}

func (d *Driver) Current(ctx context.Context, sessionID identity.SessionID) (goal.Goal, bool, error) {
	return loadGoal(ctx, d.goals, sessionID)
}

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
		if err == nil && applied {
			d.publishChange(g.SessionID())
		}
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
	applied, err := d.goals.Save(ctx, change)
	if err == nil && applied {
		d.publishChange(g.SessionID())
	}
	return applied, err
}

func (d *Driver) BeginShutdown() {
	release := d.mutations.acquireAll()
	defer release()
	d.tasks.Cancel()
}

func (d *Driver) AwaitShutdown(ctx context.Context) error {
	return d.tasks.Wait(ctx)
}
