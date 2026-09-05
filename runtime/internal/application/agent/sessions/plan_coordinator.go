package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

// PlanStore is the use case's consumer-owned persistence port.
type PlanStore interface {
	State(ctx context.Context, sessionID string) (plan.Current, error)
	Save(ctx context.Context, sessionID string, replacement plan.Replacement) error
}

// PlanClock supplies the commit time for a Plan replacement.
type PlanClock func() time.Time

// PlanCoordinator executes Plan use cases over one canonical store.
type PlanCoordinator struct {
	store         PlanStore
	now           PlanClock
	invalidations invalidation.Publish
}

// PlanDependencies is the collaborator set [NewPlanCoordinator] wires into a PlanCoordinator.
type PlanDependencies struct {
	Store         PlanStore
	Now           PlanClock
	Invalidations invalidation.Publish
}

// NewPlanCoordinator returns a Plan Coordinator. A nil Store means the optional capability is
// unavailable; callers should omit its tools and application wiring.
func NewPlanCoordinator(deps PlanDependencies) *PlanCoordinator {
	if deps.Store == nil {
		return nil
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &PlanCoordinator{store: deps.Store, now: deps.Now, invalidations: deps.Invalidations}
}

// State returns the canonical optional Plan aggregate for one session.
func (c *PlanCoordinator) State(ctx context.Context, sessionID string) (plan.Current, error) {
	if c == nil || c.store == nil {
		return plan.Current{}, errors.New("sessions: Plan store is unavailable")
	}
	if _, err := resourceid.ParseSession(sessionID); err != nil {
		return plan.Current{}, fmt.Errorf("sessions: Plan: %w", err)
	}
	state, err := c.store.State(ctx, sessionID)
	if err != nil {
		return plan.Current{}, err
	}
	if err := state.Validate(); err != nil {
		return plan.Current{}, fmt.Errorf("sessions: read invalid Plan state: %w", err)
	}
	return state, nil
}

// Replace computes and commits one complete replacement using optimistic
// concurrency. An empty steps slice clears the Plan under a new revision.
func (c *PlanCoordinator) Replace(ctx context.Context, sessionID string, steps []plan.Step) (plan.State, error) {
	replacement, err := c.PrepareReplacement(ctx, sessionID, steps)
	if err != nil {
		return plan.State{}, err
	}
	if err := c.store.Save(ctx, sessionID, replacement); err != nil {
		return plan.State{}, err
	}
	c.invalidations.Notify(invalidation.InSession(invalidation.PlanState, sessionID))
	return replacement.State(), nil
}

// PrepareReplacement decides a replacement without committing it. Cross-
// aggregate use cases use this to include the exact Plan transition in their
// own atomic write set. Steps are borrowed until the synchronous call returns;
// the decided State owns its snapshot.
func (c *PlanCoordinator) PrepareReplacement(ctx context.Context, sessionID string, steps []plan.Step) (plan.Replacement, error) {
	current, err := c.State(ctx, sessionID)
	if err != nil {
		return plan.Replacement{}, err
	}
	return c.replace(current, steps)
}

// PrepareInitial decides the first Plan state for a not-yet-created session.
// It is used when a cross-aggregate write set assigns the session identity.
func (c *PlanCoordinator) PrepareInitial(steps []plan.Step) (plan.Replacement, error) {
	if c == nil {
		return plan.Replacement{}, errors.New("sessions: Plan coordinator is unavailable")
	}
	return c.replace(plan.Current{}, steps)
}

func (c *PlanCoordinator) replace(current plan.Current, steps []plan.Step) (plan.Replacement, error) {
	next, err := current.Replace(steps, c.now())
	if err != nil {
		return plan.Replacement{}, err
	}
	return plan.NewReplacement(current.Version(), next)
}
