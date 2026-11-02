package pluginservices

import (
	"context"
	"errors"
	goalmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/goalworkflow/mcp"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame/cancellation"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func (c *CommandCalls) GoalAvailable(ctx context.Context) (bool, error) {
	contract, err := goalmcp.Contract()
	if err != nil {
		return false, err
	}
	_, found, err := plugins.SelectedContribution(ctx, c.framework, c.installations, c.selections, contract)
	return found, err
}

func (c *CommandCalls) RequireGoalAuthor(ctx context.Context, source extension.ContributionSource) error {
	if err := source.Validate(); err != nil {
		return errors.Join(plugin.ErrUnavailable, err)
	}
	contract, err := goalmcp.Contract()
	if err != nil {
		return err
	}
	_, borrow, err := workflowSession(ctx, c, contract, source)
	if err != nil {
		return err
	}
	return borrow.Close()
}

func (c *CommandCalls) DecideGoal(ctx context.Context, source extension.ContributionSource, input goalworkflow.Request) (goalworkflow.AuthoredDecision, error) {
	if source == (extension.ContributionSource{}) && input.Signal != goalworkflow.Start {
		return goalworkflow.AuthoredDecision{}, plugin.ErrUnavailable
	}
	contract, err := goalmcp.Contract()
	if err != nil {
		return goalworkflow.AuthoredDecision{}, err
	}
	location, borrow, err := workflowSession(ctx, c, contract, source)
	if err != nil {
		return goalworkflow.AuthoredDecision{}, err
	}
	defer borrow.Close()
	method, err := goalmcp.DecideMethod()
	if err != nil {
		return goalworkflow.AuthoredDecision{}, err
	}
	decision, err := query[goalworkflow.Request, goalworkflow.Decision](borrow.Context(), location, borrow.Session(), method.Name(), input, sdk.ParamsBase{})
	return goalworkflow.AuthoredDecision{Source: location.ContributionSource(), Decision: decision}, errors.Join(err, cancellation.Err(borrow.Context()))
}
