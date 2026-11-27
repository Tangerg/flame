package delivery

import (
	"context"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/project"
)

type projectBackgroundStore interface {
	Background(context.Context, identity.ProjectID, time.Time) (project.BackgroundActivity, error)
	SetBackground(context.Context, identity.ProjectID, project.BackgroundPolicy) (project.BackgroundPolicy, error)
}

const (
	ProjectsGetBackground Name = "projects.background.get"
	ProjectsSetBackground Name = "projects.background.setPolicy"
)

func registerProjectBackground(registry *Registry) {
	registry.query(MethodMeta{Name: ProjectsGetBackground, GoName: "GetProjectBackground",
		Errors: []string{protocol.ErrProjectNotFound.Error()}},
		func(service interface {
			GetProjectBackground(context.Context, protocol.GetProjectRequest) (*protocol.ProjectBackgroundActivity, error)
		}, ctx context.Context, request protocol.GetProjectRequest) (*protocol.ProjectBackgroundActivity, error) {
			return service.GetProjectBackground(ctx, request)
		})
	registry.command(MethodMeta{Name: ProjectsSetBackground, GoName: "SetProjectBackgroundPolicy",
		Errors: []string{protocol.ErrProjectNotFound.Error(), protocol.ErrRevisionConflict.Error()}},
		func(service interface {
			SetProjectBackgroundPolicy(context.Context, protocol.SetProjectBackgroundPolicyRequest) (*protocol.ProjectBackgroundPolicy, error)
		}, ctx context.Context, request protocol.SetProjectBackgroundPolicyRequest) (*protocol.ProjectBackgroundPolicy, error) {
			return service.SetProjectBackgroundPolicy(ctx, request)
		}).withOwnedReplay(atomicReplay((interface {
		commitProjectBackground(context.Context, protocol.SetProjectBackgroundPolicyRequest, atomicOutcome) error
	}).commitProjectBackground))
}

func (s *Handler) GetProjectBackground(ctx context.Context, in protocol.GetProjectRequest) (*protocol.ProjectBackgroundActivity, error) {
	id, err := wireIdentity("projectId", in.ProjectID, identity.ParseProject)
	if err != nil {
		return nil, err
	}
	state, err := s.projectBackground.Background(ctx, id, time.Now())
	if err != nil {
		return nil, wireProjectErr(err)
	}
	recent := make([]protocol.ProjectBackgroundRun, 0, len(state.Recent))
	for _, item := range state.Recent {
		recent = append(recent, protocol.ProjectBackgroundRun{
			Source:    protocol.BackgroundSource{ProjectID: id.String(), Kind: item.Kind, ID: item.ID, Version: item.Version, Title: item.Title},
			SessionID: item.SessionID, RunID: item.RunID, Status: item.Status, Outcome: item.Outcome, AdmittedAt: item.AdmittedAt,
		})
	}
	return &protocol.ProjectBackgroundActivity{Policy: presentBackgroundPolicy(state.Policy), Active: state.Active, Admitted: state.Admitted, Recent: recent}, nil
}

func (s *Handler) SetProjectBackgroundPolicy(ctx context.Context, in protocol.SetProjectBackgroundPolicyRequest) (*protocol.ProjectBackgroundPolicy, error) {
	var saved *protocol.ProjectBackgroundPolicy
	err := s.setProjectBackground(ctx, in, operation.DirectReply(&saved))
	return saved, err
}

func (s *Handler) commitProjectBackground(ctx context.Context, in protocol.SetProjectBackgroundPolicyRequest, outcome atomicOutcome) error {
	reply := atomicReply(outcome.complete, func(saved *protocol.ProjectBackgroundPolicy) ([]byte, error) {
		return encodeAtomicResult(outcome.method, saved)
	})
	return outcome.settleRefusal(ctx, s.setProjectBackground(ctx, in, reply))
}

func (s *Handler) setProjectBackground(ctx context.Context, in protocol.SetProjectBackgroundPolicyRequest, reply operation.Reply[*protocol.ProjectBackgroundPolicy]) error {
	id, err := wireIdentity("projectId", in.ProjectID, identity.ParseProject)
	if err != nil {
		return err
	}
	value := project.BackgroundPolicy(in.Policy)
	if err := value.Validate(); err != nil {
		return wireProjectErr(err)
	}
	return reply(ctx, func(tx context.Context) (*protocol.ProjectBackgroundPolicy, error) {
		next, err := s.projectBackground.SetBackground(tx, id, value)
		if err != nil {
			return nil, wireProjectErr(err)
		}
		view := presentBackgroundPolicy(next)
		return &view, nil
	})
}

func presentBackgroundPolicy(value project.BackgroundPolicy) protocol.ProjectBackgroundPolicy {
	return protocol.ProjectBackgroundPolicy(value)
}
