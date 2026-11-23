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

type planStore interface {
	State(ctx context.Context, sessionID identity.SessionID) (plan.Current, error)
	Save(ctx context.Context, sessionID identity.SessionID, replacement plan.Replacement) error
}

type PlanAuthor interface {
	AuthorPlan(context.Context, extension.ContributionSource, planworkflow.Proposal) (planworkflow.AuthoredDocument, error)
	ReviewPlan(context.Context, extension.ContributionSource, planworkflow.Document) (string, error)
}

type PlanClock func() time.Time

type PlanCoordinator struct {
	store         planStore
	author        PlanAuthor
	now           PlanClock
	invalidations invalidation.Publish
}

type PlanDependencies struct {
	Store         planStore
	Author        PlanAuthor
	Now           PlanClock
	Invalidations invalidation.Publish
}

func NewPlanCoordinator(deps PlanDependencies) (*PlanCoordinator, error) {
	if lo.IsNil(deps.Store) {
		return nil, errors.New("sessions: Plan store is required")
	}
	if deps.Now == nil {
		return nil, errors.New("sessions: Plan clock is required")
	}
	if lo.IsNil(deps.Author) {
		return nil, errors.New("sessions: Plan author is required")
	}
	if deps.Invalidations == nil {
		return nil, errors.New("sessions: Plan invalidations are required")
	}
	return &PlanCoordinator{store: deps.Store, author: deps.Author, now: deps.Now, invalidations: deps.Invalidations}, nil
}

func (c *PlanCoordinator) State(ctx context.Context, sessionID identity.SessionID) (plan.Current, error) {
	if err := sessionID.Validate(); err != nil {
		return plan.Current{}, fmt.Errorf("sessions: Plan: %w", err)
	}
	return c.store.State(ctx, sessionID)
}

func (c *PlanCoordinator) Replace(ctx context.Context, sessionID identity.SessionID, steps []plan.Step) (plan.State, error) {
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

func (c *PlanCoordinator) prepareAuthorDecision(ctx context.Context, sessionID identity.SessionID, doc plan.Document) (plan.Replacement, error) {
	current, err := c.State(ctx, sessionID)
	if err != nil {
		return plan.Replacement{}, err
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
	document, err := planworkflow.Seal(plan.ProposalOf(state.Steps()))
	if err != nil {
		return "", err
	}
	if state.Document().Source == (extension.ContributionSource{}) {
		return "", plugin.ErrUnavailable
	}
	return c.author.ReviewPlan(ctx, state.Document().Source, document)
}

func (c *PlanCoordinator) PrepareReplacement(ctx context.Context, sessionID identity.SessionID, steps []plan.Step) (plan.Replacement, error) {
	current, err := c.State(ctx, sessionID)
	if err != nil {
		return plan.Replacement{}, err
	}
	return c.reference(current, plan.Document{Steps: steps})
}

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
