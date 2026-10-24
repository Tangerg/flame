package execution

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	modelchoice "github.com/Tangerg/flame/runtime/model"
)

type modelCalls struct {
	session *interactionSession
	member  runs.ExecutorMember
}

func (i *interactionSession) modelCallContext(ctx context.Context, member runs.ExecutorMember, owner string) context.Context {
	ctx = executionctx.WithInvocationOwner(ctx, owner)
	return executionctx.WithModelCalls(ctx, modelCalls{session: i, member: member})
}

func (m modelCalls) Start(ctx context.Context, id string, selection modelchoice.Selection) (func(context.Context, error) error, error) {
	if err := selection.ValidateExact(); err != nil {
		return nil, err
	}
	if err := m.session.commitFact(ctx, m.member, runs.ModelCallStarted{CallID: id, Purpose: run.ModelIndependent}); err != nil {
		return nil, err
	}
	return func(ctx context.Context, cause error) error {
		publication, cancel := m.session.lifetime.publicationContext(ctx)
		defer cancel()
		source, found, err := m.session.modelEvidence.EndpointForCaller(publication, id)
		if err != nil {
			return err
		}
		if !found {
			if cause == nil {
				return errors.New("execution: independent model completion has no physical evidence")
			}
			return m.session.commitFact(publication, m.member, runs.ModelCallFailed{CallID: id})
		}
		if source.RunID == "" || source.SegmentID == "" || source.Invocation.Caller.ID != id || source.SessionID != m.session.scope.SessionID || source.Invocation.Provider != selection.Provider() || source.Invocation.Model != selection.Model() {
			return errors.New("execution: independent model evidence differs from admitted call")
		}
		completed, err := accountModelEndpoint(id, source)
		if err != nil {
			return err
		}
		if cause != nil || source.Response == nil {
			return m.session.commitFact(publication, m.member, runs.ModelCallFailed{CallID: id, KnownResponse: source.Response, Accounting: &completed})
		}
		return m.session.commitFact(publication, m.member, completed)
	}, nil
}

func accountModelEndpoint(callID string, source modelref.EndpointObservation) (runs.ModelCallCompleted, error) {
	if source.Response != nil {
		if err := source.Response.Validate(); err != nil {
			return runs.ModelCallCompleted{}, err
		}
	} else if source.Usage != nil || source.CostUSD != nil {
		return runs.ModelCallCompleted{}, errors.New("execution: unknown model outcome carries reported consumption")
	}
	cost, err := accounting.CostFromOptional(source.CostUSD)
	if err != nil {
		return runs.ModelCallCompleted{}, err
	}
	consumption := runs.ModelCallCompleted{CallID: callID, Model: source.ServedModel, Cost: cost}
	if source.Usage != nil {
		usage := source.Usage.Clone()
		consumption.ReportedUsage = &usage
	}
	return consumption, nil
}
