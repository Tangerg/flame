package pluginservices

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/codeintel"
	"github.com/Tangerg/flame/qimu/cancellation"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func (c *CommandCalls) AcquireCodeIntelligence(ctx context.Context) (toolset.CodeIntelligenceUse, error) {
	contract, err := codeintel.Contract()
	if err != nil {
		return nil, err
	}
	_, selected, err := c.selections.Get(ctx, contract.Descriptor().Reference)
	if err != nil || !selected {
		return nil, err
	}
	binding, borrowed, err := contributionSession(ctx, c.framework, c.installations, c.selections, c.pool, contract)
	if errors.Is(err, errInactive) {
		return nil, plugin.ErrUnavailable
	}
	if err != nil {
		return nil, err
	}
	activation, err := borrowed.Activation()
	if err == nil {
		activation, err = plugins.OriginalActivation([]plugin.Activation{activation}, binding.InstallationID, binding.Digest, binding.ServerName)
	}
	if err != nil {
		return nil, errors.Join(err, borrowed.Close())
	}
	binding.Activation = &activation
	return &codeIntelligenceUse{binding: binding, session: borrowed}, nil
}

type codeIntelligenceUse struct {
	binding plugins.ContributionBinding
	session *mcp.BorrowedSession
}

func (u *codeIntelligenceUse) Format(document codeintel.Document) (codeintel.Transformation, error) {
	if err := document.Validate(); err != nil {
		return codeintel.Transformation{}, err
	}
	method, err := codeintel.FormatMethod()
	if err != nil {
		return codeintel.Transformation{}, err
	}
	result, err := query[codeintel.Document, codeintel.Transformation](u.session.Context(), u.binding, u.session.Session(), method.Name(), document, sdk.ParamsBase{})
	if err == nil {
		err = result.Validate(document)
	}
	return result, errors.Join(err, cancellation.Err(u.session.Context()))
}

func (u *codeIntelligenceUse) Inspect(input codeintel.Inspection) (codeintel.Report, error) {
	if err := input.Validate(); err != nil {
		return codeintel.Report{}, err
	}
	method, err := codeintel.InspectMethod()
	if err != nil {
		return codeintel.Report{}, err
	}
	result, err := query[codeintel.Inspection, codeintel.Report](u.session.Context(), u.binding, u.session.Session(), method.Name(), input, sdk.ParamsBase{})
	if err == nil {
		err = result.Validate(input.Document)
	}
	return result, errors.Join(err, cancellation.Err(u.session.Context()))
}

func (u *codeIntelligenceUse) Close() error { return u.session.Close() }
