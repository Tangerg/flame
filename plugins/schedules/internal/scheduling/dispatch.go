package scheduling

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/plugins/schedules/internal/schedule"
	"github.com/samber/lo"
)

type RunReference struct {
	SessionID string `json:"sessionId"`
	RunID     string `json:"runId"`
}

func NewRunReference(request schedule.RunRequest) RunReference {
	return RunReference{SessionID: request.SessionID().String(), RunID: request.RunID().String()}
}

type RunAdmission interface {
	Admit(context.Context, schedule.RunRequest) error
}

type DispatchStore interface {
	CapturedRun(context.Context, identity.RunID) (schedule.RunRequest, error)
	AcceptRun(context.Context, identity.RunID) error
}

type Dispatcher struct {
	store DispatchStore
	runs  RunAdmission
}

func NewDispatcher(store DispatchStore, runs RunAdmission) (*Dispatcher, error) {
	if lo.IsNil(store) || lo.IsNil(runs) {
		return nil, errors.New("schedules: dispatch store and Run admission are required")
	}
	return &Dispatcher{store: store, runs: runs}, nil
}

func (d *Dispatcher) Dispatch(ctx context.Context, id identity.RunID) (RunReference, error) {
	request, err := d.store.CapturedRun(ctx, id)
	if err != nil {
		return RunReference{}, err
	}
	if err := request.Validate(); err != nil {
		return RunReference{}, err
	}
	if request.RunID() != id {
		return RunReference{}, errors.New("schedules: captured dispatch identity disagrees")
	}
	if err := d.runs.Admit(ctx, request); err != nil {
		return RunReference{}, err
	}
	if err := d.store.AcceptRun(ctx, id); err != nil {
		return RunReference{}, err
	}
	return NewRunReference(request), nil
}
