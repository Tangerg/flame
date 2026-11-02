package pluginservices

import (
	"context"
	"errors"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	planmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/planworkflow/mcp"

	"github.com/Tangerg/flame/packages/go-sdk/planworkflow"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/frame/cancellation"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func (c *CommandCalls) PlanAvailable(ctx context.Context) (bool, error) {
	contract, err := planmcp.Contract()
	if err != nil {
		return false, err
	}
	_, found, err := plugins.SelectedContribution(ctx, c.framework, c.installations, c.selections, contract)
	return found, err
}

func (c *CommandCalls) AuthorPlan(ctx context.Context, source extension.ContributionSource, proposal planworkflow.Proposal) (planworkflow.AuthoredDocument, error) {
	contract, err := planmcp.Contract()
	if err != nil {
		return planworkflow.AuthoredDocument{}, err
	}
	location, borrow, err := workflowSession(ctx, c, contract, source)
	if err != nil {
		return planworkflow.AuthoredDocument{}, err
	}
	defer borrow.Close()
	method, err := planmcp.AuthorMethod()
	if err != nil {
		return planworkflow.AuthoredDocument{}, err
	}
	decision, err := query[planworkflow.Proposal, planworkflow.Decision](borrow.Context(), location, borrow.Session(), method.Name(), proposal, sdk.ParamsBase{})
	if err = errors.Join(err, cancellation.Err(borrow.Context())); err != nil {
		return planworkflow.AuthoredDocument{}, err
	}
	document, err := decision.Open()
	return planworkflow.AuthoredDocument{Document: document, Source: location.ContributionSource()}, err
}

func (c *CommandCalls) ReviewPlan(ctx context.Context, source extension.ContributionSource, document planworkflow.Document) (string, error) {
	contract, err := planmcp.Contract()
	if err != nil {
		return "", err
	}
	location, borrow, err := workflowSession(ctx, c, contract, source)
	if err != nil {
		return "", err
	}
	defer borrow.Close()
	method, err := planmcp.ReviewMethod()
	if err != nil {
		return "", err
	}
	text, err := query[planworkflow.Document, string](borrow.Context(), location, borrow.Session(), method.Name(), document, sdk.ParamsBase{})
	return text, errors.Join(err, cancellation.Err(borrow.Context()))
}
