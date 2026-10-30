package execution

import (
	"context"
	"errors"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
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
		record := source.Snapshot()
		if record.RunID == "" || record.SegmentID == "" || record.Invocation.Caller.ID != id || record.SessionID != m.session.scope.SessionID || record.Invocation.Provider != selection.Provider() || record.Invocation.Model != selection.Model() {
			return errors.New("execution: independent model evidence differs from admitted call")
		}
		completed, err := accountModelEndpoint(id, source)
		if err != nil {
			return err
		}
		if cause != nil || source.Response() == nil {
			return m.session.commitFact(publication, m.member, runs.ModelCallFailed{CallID: id, KnownResponse: source.Response(), Accounting: &completed})
		}
		return m.session.commitFact(publication, m.member, completed)
	}, nil
}

func accountModelEndpoint(callID string, source modelref.EndpointObservation) (runs.ModelCallCompleted, error) {
	if err := source.Validate(); err != nil {
		return runs.ModelCallCompleted{}, err
	}
	record := source.Snapshot()
	cost, err := accounting.CostFromOptional(record.CostUSD)
	if err != nil {
		return runs.ModelCallCompleted{}, err
	}
	consumption := runs.ModelCallCompleted{CallID: callID, Model: source.ServedModel(), Cost: cost}
	if record.Usage != nil {
		usage := record.Usage.Clone()
		consumption.ReportedUsage = &usage
	}
	return consumption, nil
}
