// Package plans owns the application use cases for reading and replacing a
// session's execution Plan. Domain state decides each replacement; persistence
// only compares the expected revision and saves that decided state.
package plans

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/plan"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

// Store is the use case's consumer-owned persistence port.
type Store interface {
	State(ctx context.Context, sessionID string) (plan.Current, error)
	Save(ctx context.Context, sessionID string, expected plan.Version, replacement plan.State) error
}

// Clock supplies the commit time for a Plan replacement.
type Clock func() time.Time

// Coordinator executes Plan use cases over one canonical store.
type Coordinator struct {
	store         Store
	now           Clock
	invalidations invalidation.Publish
}

// Dependencies is the collaborator set [New] wires into a Coordinator.
type Dependencies struct {
	Store         Store
	Now           Clock
	Invalidations invalidation.Publish
}

// New returns a Plan Coordinator. A nil Store means the optional capability is
// unavailable; callers should omit its tools and application wiring.
func New(deps Dependencies) *Coordinator {
	if deps.Store == nil {
		return nil
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Coordinator{store: deps.Store, now: deps.Now, invalidations: deps.Invalidations}
}

// State returns the canonical optional Plan aggregate for one session.
func (c *Coordinator) State(ctx context.Context, sessionID string) (plan.Current, error) {
	if c == nil || c.store == nil {
		return plan.Current{}, errors.New("plans: store is unavailable")
	}
	if _, err := resourceid.ParseSession(sessionID); err != nil {
		return plan.Current{}, fmt.Errorf("plans: %w", err)
	}
	state, err := c.store.State(ctx, sessionID)
	if err != nil {
		return plan.Current{}, err
	}
	if err := state.Validate(); err != nil {
		return plan.Current{}, fmt.Errorf("plans: read invalid state: %w", err)
	}
	return state, nil
}

// Replace computes and commits one complete replacement using optimistic
// concurrency. An empty steps slice clears the Plan under a new revision.
func (c *Coordinator) Replace(ctx context.Context, sessionID string, steps []plan.Step) (plan.State, error) {
	replacement, err := c.PrepareReplacement(ctx, sessionID, steps)
	if err != nil {
		return plan.State{}, err
	}
	if err := c.store.Save(ctx, sessionID, replacement.ExpectedVersion(), replacement.State()); err != nil {
		return plan.State{}, err
	}
	c.invalidations.Notify(invalidation.InSession(invalidation.PlanState, sessionID))
	return replacement.State(), nil
}

// PrepareReplacement decides a replacement without committing it. Cross-
// aggregate use cases use this to include the exact Plan transition in their
// own atomic write set.
func (c *Coordinator) PrepareReplacement(ctx context.Context, sessionID string, steps []plan.Step) (Replacement, error) {
	current, err := c.State(ctx, sessionID)
	if err != nil {
		return Replacement{}, err
	}
	return c.replace(current, steps)
}

// PrepareInitial decides the first Plan state for a not-yet-created session.
// It is used when a cross-aggregate write set assigns the session identity.
func (c *Coordinator) PrepareInitial(steps []plan.Step) (Replacement, error) {
	if c == nil {
		return Replacement{}, errors.New("plans: coordinator is unavailable")
	}
	return c.replace(plan.Current{}, steps)
}

func (c *Coordinator) replace(current plan.Current, steps []plan.Step) (Replacement, error) {
	next, err := current.Replace(steps, c.now())
	if err != nil {
		return Replacement{}, err
	}
	return newReplacement(current.Version(), next)
}

// Replacement is an immutable, application-decided Plan state transition.
// Persistence implementations may execute it but may not enrich or reinterpret it.
type Replacement struct {
	expectedVersion plan.Version
	state           plan.State
}

func newReplacement(expectedVersion plan.Version, state plan.State) (Replacement, error) {
	replacement := Replacement{expectedVersion: expectedVersion, state: state}
	if err := replacement.Validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

// ExpectedVersion returns the optional state identity this replacement was based on.
func (r Replacement) ExpectedVersion() plan.Version { return r.expectedVersion }

// State returns the already-decided replacement state.
func (r Replacement) State() plan.State {
	return r.state
}

// Validate verifies that the replacement advances its expected revision once.
func (r Replacement) Validate() error {
	if err := r.expectedVersion.AdvancesTo(r.state); err != nil {
		return fmt.Errorf("plans: invalid replacement: %w", err)
	}
	return nil
}
