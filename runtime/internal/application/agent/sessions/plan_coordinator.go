package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/planworkflow"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/samber/lo"
)

// planStore is the use case's consumer-owned persistence port.
type planStore interface {
	State(ctx context.Context, sessionID string) (plan.Current, error)
	Save(ctx context.Context, sessionID string, replacement plan.Replacement) error
}

type PlanAuthor interface {
	AuthorPlan(context.Context, extension.ContributionSource, planworkflow.Proposal) (planworkflow.AuthoredDocument, error)
	ReviewPlan(context.Context, extension.ContributionSource, planworkflow.Document) (string, error)
}

// PlanClock supplies the commit time for a Plan replacement.
type PlanClock func() time.Time

// PlanCoordinator executes Plan use cases over one canonical store.
type PlanCoordinator struct {
	store         planStore
	author        PlanAuthor
	now           PlanClock
	invalidations invalidation.Publish
}

// PlanDependencies is the collaborator set [NewPlanCoordinator] wires into a PlanCoordinator.
type PlanDependencies struct {
	Store         planStore
	Author        PlanAuthor
	Now           PlanClock
	Invalidations invalidation.Publish
}

// NewPlanCoordinator constructs the Plan use cases over a required store.
func NewPlanCoordinator(deps PlanDependencies) (*PlanCoordinator, error) {
	if lo.IsNil(deps.Store) {
		return nil, errors.New("sessions: Plan store is required")
	}
	if deps.Now == nil {
		return nil, errors.New("sessions: Plan clock is required")
	}
	return &PlanCoordinator{store: deps.Store, author: deps.Author, now: deps.Now, invalidations: deps.Invalidations}, nil
}

// State returns the canonical optional Plan aggregate for one session.
func (c *PlanCoordinator) State(ctx context.Context, sessionID string) (plan.Current, error) {
	if err := identity.ValidateSession(sessionID); err != nil {
		return plan.Current{}, fmt.Errorf("sessions: Plan: %w", err)
	}
	return c.store.State(ctx, sessionID)
}

// Replace computes and commits one complete replacement using optimistic
// concurrency. An empty steps slice clears the Plan under a new revision.
func (c *PlanCoordinator) Replace(ctx context.Context, sessionID string, steps []plan.Step) (plan.State, error) {
	replacement, err := c.prepareAuthorDecision(ctx, sessionID, plan.Document{Steps: steps})
	if err != nil {
		return plan.State{}, err
	}
	if err := c.store.Save(ctx, sessionID, replacement); err != nil {
		return plan.State{}, err
	}
	c.invalidations.Notify(invalidation.InSession(invalidation.Plan, sessionID))
	return replacement.State(), nil
}

func (c *PlanCoordinator) prepareAuthorDecision(ctx context.Context, sessionID string, doc plan.Document) (plan.Replacement, error) {
	current, err := c.State(ctx, sessionID)
	if err != nil {
		return plan.Replacement{}, err
	}
	if c.author == nil {
		return plan.Replacement{}, fmt.Errorf("plan: author unavailable: %w", plugin.ErrUnavailable)
	}
	document, err := c.author.AuthorPlan(ctx, doc.Source, plan.ProposalOf(doc.Steps))
	if err != nil {
		return plan.Replacement{}, err
	}
	if err := document.Verify(); err != nil {
		return plan.Replacement{}, err
	}
	if doc.Source != (extension.ContributionSource{}) && doc.Source != document.Source {
		return plan.Replacement{}, errors.New("plan: original author changed")
	}
	if err := document.Source.Validate(); err != nil {
		return plan.Replacement{}, err
	}
	return c.reference(current, plan.Document{Source: document.Source, Steps: plan.StepsOf(document.Proposal)})
}

func (c *PlanCoordinator) Review(ctx context.Context, state plan.State) (string, error) {
	if c.author == nil {
		return "", plugin.ErrUnavailable
	}
	document, err := planworkflow.Seal(plan.ProposalOf(state.Steps()))
	if err != nil {
		return "", err
	}
	if state.Document().Source == (extension.ContributionSource{}) {
		return "", plugin.ErrUnavailable
	}
	return c.author.ReviewPlan(ctx, state.Document().Source, document)
}

// PrepareReplacement decides a replacement without committing it. Cross-
// aggregate use cases use this to include the exact Plan transition in their
// own atomic write set. Steps are borrowed until the synchronous call returns;
// the decided State owns its snapshot.
//
// Persistence receives the decided replacement and may only apply its CAS; it
// never assigns a revision or update time itself.
func (c *PlanCoordinator) PrepareReplacement(ctx context.Context, sessionID string, steps []plan.Step) (plan.Replacement, error) {
	current, err := c.State(ctx, sessionID)
	if err != nil {
		return plan.Replacement{}, err
	}
	return c.reference(current, plan.Document{Steps: steps})
}

// PrepareInitial decides the first Plan state for a not-yet-created session.
// It is used when a cross-aggregate write set assigns the session identity.
func (c *PlanCoordinator) PrepareInitial(steps []plan.Step) (plan.Replacement, error) {
	return c.reference(plan.Current{}, plan.Document{Steps: steps})
}

func (c *PlanCoordinator) reference(current plan.Current, doc plan.Document) (plan.Replacement, error) {
	next, err := current.Reference(doc, c.now())
	if err != nil {
		return plan.Replacement{}, err
	}
	return plan.NewReplacement(current.Version(), next)
}
